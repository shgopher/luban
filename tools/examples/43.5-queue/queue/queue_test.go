package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNewQueue verifies that newQueue initializes the structure correctly.
func TestNewQueue(t *testing.T) {
	q := newQueue()

	if q == nil {
		t.Errorf("newQueue returned nil, expected a pointer to queue")
	}
	if q.innerChan == nil {
		t.Errorf("innerChan was not initialized")
	}
	if cap(q.innerChan) != 1 {
		t.Errorf("innerChan capacity was %d, expected 1", cap(q.innerChan))
	}
	if q.queueTasks == nil {
		t.Errorf("queueTasks was not initialized")
	}
	if q.queueTasks.Len() != 0 {
		t.Errorf("queueTasks was not empty, expected 0 elements")
	}
}

// TestQueuePushPop verifies that push and pop operations are correct.
func TestQueuePushPop(t *testing.T) {
	q := newQueue()
	task1 := &Task{ID: 1, Data: "Task 1"}
	task2 := &Task{ID: 2, Data: "Task 2"}

	// Check pop from empty queue
	if poppedTask := q.pop(); poppedTask != nil {
		t.Errorf("Pop from empty queue returned %v, expected nil", poppedTask)
	}

	// Push task1
	q.push(task1)
	if q.queueTasks.Len() != 1 {
		t.Errorf("After push, queue length was %d, expected 1", q.queueTasks.Len())
	}

	// Pop task1
	poppedTask := q.pop()
	if poppedTask == nil || poppedTask.ID != 1 {
		t.Errorf("Pop returned %v, expected task1", poppedTask)
	}
	if q.queueTasks.Len() != 0 {
		t.Errorf("After pop, queue length was %d, expected 0", q.queueTasks.Len())
	}

	// Push task1 and task2, then pop in order
	q.push(task1)
	q.push(task2)
	if q.queueTasks.Len() != 2 {
		t.Errorf("After two pushes, queue length was %d, expected 2", q.queueTasks.Len())
	}

	poppedTask = q.pop()
	if poppedTask == nil || poppedTask.ID != 1 {
		t.Errorf("First pop returned %v, expected task1", poppedTask)
	}
	poppedTask = q.pop()
	if poppedTask == nil || poppedTask.ID != 2 {
		t.Errorf("Second pop returned %v, expected task2", poppedTask)
	}
	if q.queueTasks.Len() != 0 {
		t.Errorf("After all pops, queue length was %d, expected 0", q.queueTasks.Len())
	}
}

// TestInpProcessBasicFlow tests the basic flow of work inpProcess.
func TestInpProcessBasicFlow(t *testing.T) {
	inp := make(chan *Task, 5)
	q := InpQueue(inp) // inpProcess starts here

	// We send several tasks
	for i := range 3 {
		inp <- &Task{ID: i}
	}
	time.Sleep(10 * time.Millisecond) // Give time to the inpProcess goroutine to process tasks

	if q.queueTasks.Len() != 3 {
		t.Errorf("Expected 3 tasks in queue, got %d", q.queueTasks.Len())
	}

	// Close the input channel so that inpProcess can complete
	close(inp)
	time.Sleep(10 * time.Millisecond) // Give inpProcess time to complete

	select {
	case <-q.innerChan: // Read from buffer
	default:
	}
	select {
	case _, ok := <-q.innerChan:
		if ok {
			t.Errorf("innerChan was not closed by inpProcess")
		}
	default:
	}
}

// TestOutProcessBasicFlow tests the basic workflow outProcess.
func TestOutProcessBasicFlow(t *testing.T) {
	q := newQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := OutQueue(ctx, q) // outProcess starts here

	// Put tasks directly into the queue (simulate inpProcess)
	task1 := &Task{ID: 1}
	task2 := &Task{ID: 2}
	q.push(task1)
	q.push(task2)

	// Signal outProcess about the presence of tasks
	select {
	case q.innerChan <- struct{}{}:
	default:
	}

	receivedTasks := []*Task{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2; i++ {
			select {
			case t := <-out:
				receivedTasks = append(receivedTasks, t)
			case <-time.After(100 * time.Millisecond):
				t.Error("Timed out waiting for task from out channel")
				return
			}
		}
	}()

	wg.Wait()

	if len(receivedTasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(receivedTasks))
	}
	if receivedTasks[0].ID != 1 || receivedTasks[1].ID != 2 {
		t.Errorf("Tasks received in wrong order: %v", receivedTasks)
	}

	// Check that outProcess completes when the queue is empty.
	close(q.innerChan)

	// Give time for outProcess to complete
	time.Sleep(10 * time.Millisecond)

	// Make sure out is closed
	_, ok := <-out
	if ok {
		t.Errorf("out channel was not closed by outProcess")
	}
}

// TestPipelineCompletion tests end-to-end pipeline completion.
func TestPipelineCompletion(t *testing.T) {
	inp := make(chan *Task, 5)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := InpQueue(inp)
	out := OutQueue(ctx, q)

	expectedTasks := 5
	for i := 0; i < expectedTasks; i++ {
		inp <- &Task{ID: i, Data: fmt.Sprintf("Data %d", i)}
	}
	close(inp) // Close the input channel

	receivedCount := 0
	for range out { // Read from the output channel until it closes
		receivedCount++
	}

	if receivedCount != expectedTasks {
		t.Errorf("Expected %d tasks, got %d", expectedTasks, receivedCount)
	}

	// Make sure all goroutines have completed
	time.Sleep(50 * time.Millisecond)

	select {
	case <-q.innerChan: // Read from buffer
	default:
	}
	select {
	case _, ok := <-q.innerChan:
		if ok {
			t.Errorf("innerChan was not closed")
		}
	default:
	}
	select {
	case _, ok := <-out:
		if ok {
			t.Errorf("out was not closed")
		}
	default:
	}
}

// TestPipelineCancellation checks pipeline cancellation via context.
func TestPipelineCancellation(t *testing.T) {
	inp := make(chan *Task, 10)
	ctx, cancel := context.WithCancel(context.Background())

	q := InpQueue(inp)
	out := OutQueue(ctx, q)

	// Start a goroutine that will send tasks but not close the channel
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for i := 0; i < 100; i++ { // We send a lot of tasks
			inp <- &Task{ID: i}
			time.Sleep(1 * time.Millisecond)
		}
		wg.Done()
	}()

	// We wait a bit so that the tasks have time to get into the queue and outProcess starts working
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Give goroutines time to complete after cancellation
	time.Sleep(50 * time.Millisecond)

	select {
	case _, ok := <-out:
		if ok {
			t.Errorf("out channel is still open after context cancellation")
		}
	default:
	}

	// Make sure inpProcess keeps running until inp is closed
	// (even though outProcess has already finished)
	// Add another task and make sure it gets into the queue
	inp <- &Task{ID: 999}
	time.Sleep(10 * time.Millisecond)
	// We can't check how many tasks have been processed because out is closed.
	// It's important that outProcess _has completed_.
	if q.queueTasks.Len() == 0 {
		t.Errorf("Expected some tasks to remain in queue after cancellation, got 0")
	}
	// Close inp so that inpProcess also terminates
	wg.Wait()
	close(inp)
	time.Sleep(10 * time.Millisecond)

	select {
	case <-q.innerChan: // Read from buffer
	default:
	}
	select {
	case _, ok := <-q.innerChan:
		if ok {
			t.Errorf("innerChan was not closed")
		}
	default:
	}
}

// TestSlowConsumerFastProducer checks that the queue smoothes out speed differences.
func TestSlowConsumerFastProducer(t *testing.T) {
	inp := make(chan *Task, 100) // Buffer for producer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := InpQueue(inp)
	out := OutQueue(ctx, q)

	numTasks := 20
	var wg sync.WaitGroup
	wg.Add(1)

	// Producer: quickly sends tasks
	go func() {
		defer wg.Done()
		for i := 0; i < numTasks; i++ {
			inp <- &Task{ID: i, Data: fmt.Sprintf("Data %d", i)}
			time.Sleep(5 * time.Millisecond) // Very fast
		}
		close(inp)
	}()

	// Consumer: Processes tasks slowly
	receivedCount := 0
	for range out {
		receivedCount++
		time.Sleep(50 * time.Millisecond) // Very slowly
	}

	wg.Wait() // Wait for the producer to complete

	if receivedCount != numTasks {
		t.Errorf("Expected %d tasks, got %d", numTasks, receivedCount)
	}

	// Here we just make sure that the pipeline terminated correctly,
	// and that the internal queue mechanisms dealt with the imbalance.
	select {
	case <-q.innerChan: // Read from buffer
	default:
	}
	select {
	case _, ok := <-q.innerChan:
		if ok {
			t.Errorf("innerChan was not closed after all tasks processed")
		}
	default:
	}
}
