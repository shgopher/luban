package main

import (
	"context"
	"fmt"
	"time"

	"github.com/onexstack/go-sample-queue/queue"
)

func main() {
	startTime := time.Now()
	// Initialize context to manage the entire pipeline
	mainCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Create an initial input channel for tasks
	// In a real system, we get it from the previous pipeline element.
	inpChan := make(chan *queue.Task)

	// 2. Embed our queue into the pipeline:
	// inpChan -> inpQueue (transforms channel to queue) -> outQueue (transforms queue to channel) -> outChan
	// This stage simulates some processing and includes the queue.
	outChan := queue.OutQueue(mainCtx, queue.InpQueue(inpChan))

	// 3. Start a producer goroutine:
	// It will generate tasks and send them to inpChan.
	produced := 0
	go func() {
		fmt.Printf("Producer: started. (%dms)\n", time.Since(startTime).Milliseconds())
		for i := range 5 {
			task := &queue.Task{ID: i, Data: fmt.Sprintf("Task #%d", i)}
			fmt.Printf("Producer: Sending %s  (%dms)\n", task.Data, time.Since(startTime).Milliseconds())
			inpChan <- task
			produced++
			time.Sleep(200 * time.Millisecond) // Simulate producer work
		}
		close(inpChan) // Important: close the input channel when all tasks are sent
		fmt.Printf("Producer: All tasks sent, input channel closed. (%dms)\n", time.Since(startTime).Milliseconds())
	}()

	// 4. Start a consumer goroutine:
	// It will read tasks from outChan (the output of our queue).
	consumed := 0
	go func() {
		fmt.Printf("Consumer: started. (%dms)\n", time.Since(startTime).Milliseconds())
		for task := range outChan {
			consumed++
			fmt.Printf("Consumer: Received %s  (%dms)\n", task.Data, time.Since(startTime).Milliseconds())
			time.Sleep(400 * time.Millisecond) // Simulate a slower consumer
		}
		fmt.Printf("Consumer: All tasks processed, output channel closed. (%dms)\n", time.Since(startTime).Milliseconds())
	}()
	// The pipeline will finish when inpChan closes -> inpProcess finishes ->
	// queue.innerChan closes -> outProcess finishes -> outChan closes.

	/*
	    // Uncomment this code to see how context manages the operation's lifecycle.
	   	time.Sleep(1 * time.Second) // Timeout in case of hang
	   	fmt.Printf("Main: Timeout reached, cancelling context. (%dms)\n", time.Since(startTime).Milliseconds())
	   	cancel()
	*/
	// Small delay for all goroutines to finish after cancellation/completion.
	time.Sleep(3 * time.Second)
	fmt.Printf("-produced: %d tasks, -consumed: %d tasks.\n", produced, consumed)
	fmt.Printf("Main: Application finished. (%dms)\n", time.Since(startTime).Milliseconds())
}
