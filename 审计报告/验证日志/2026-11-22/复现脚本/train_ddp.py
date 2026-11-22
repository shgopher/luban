#!/usr/bin/env python3
"""Minimal but real DDP training script for book verification (CPU + gloo).

Two modes:
  python train_ddp.py --mode ddp      -> DistributedDataParallel, gradients AllReduced
  python train_ddp.py --mode noddp    -> identical model/data/optimizer, NO sync

Evidence printed by every rank:
  * WORLD_SIZE / RANK / LOCAL_RANK / backend / torch version / cuda available
  * number of samples in this rank's shard (DistributedSampler)
  * per-step loss, plus an all_reduced mean loss (so both ranks print the same number)
  * grad L2 norm of layer1.weight + first 3 numbers of grad[0][0][:3]
  * parameter L2 norm after training (identical iff weights stayed in sync)
"""
import argparse
import os
import time

import torch
import torch.distributed as dist
import torch.multiprocessing as mp          # noqa: F401  (torchrun spawns processes)
import torch.nn as nn
from torch.utils.data import DataLoader, Dataset, DistributedSampler

N_SAMPLES = 64
IN_DIM = 32
HIDDEN = 64


class ToyRegression(Dataset):
    """Deterministic synthetic dataset: y = x @ w_true + b_true."""

    def __init__(self, n=N_SAMPLES, dim=IN_DIM, seed=1234):
        g = torch.Generator().manual_seed(seed)
        self.x = torch.randn(n, dim, generator=g)
        w = torch.randn(dim, 1, generator=g)
        self.y = self.x @ w + 0.01 * torch.randn(n, 1, generator=g)

    def __len__(self):
        return len(self.x)

    def __getitem__(self, i):
        return self.x[i], self.y[i]


def build_model(seed=0):
    # Same seed on every rank => identical initial weights, so any difference in
    # gradients/params afterwards is caused by data/communication only.
    torch.manual_seed(seed)
    return nn.Sequential(
        nn.Linear(IN_DIM, HIDDEN),
        nn.ReLU(),
        nn.Linear(HIDDEN, 1),
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--mode", choices=["ddp", "noddp"], default="ddp")
    ap.add_argument("--steps", type=int, default=5)
    ap.add_argument("--batch-size", type=int, default=8)
    ap.add_argument("--lr", type=float, default=0.05)
    ap.add_argument("--master-port", type=int, default=29500)
    args = ap.parse_args()

    backend = None
    rank = int(os.environ.get("RANK", 0))
    world = int(os.environ.get("WORLD_SIZE", 1))
    local_rank = int(os.environ.get("LOCAL_RANK", 0))

    if args.mode == "ddp":
        backend = "gloo"                     # CPU-only machine: no NCCL, no GPU
        dist.init_process_group(backend=backend, init_method="env://",
                                world_size=world, rank=rank)
        rank = dist.get_rank()
        world = dist.get_world_size()
        local_rank = int(os.environ.get("LOCAL_RANK", 0))

    dev = torch.device("cpu")
    model = build_model().to(dev)
    if args.mode == "ddp":
        # device_ids must stay None on CPU; the book snippet's device_ids=[local_rank]
        # is GPU-only and raises on a CPU build.
        model = nn.parallel.DistributedDataParallel(model, device_ids=None)

    ds = ToyRegression()
    if args.mode == "ddp":
        sampler = DistributedSampler(ds, num_replicas=world, rank=rank,
                                     shuffle=False, drop_last=False)
        sampler.set_epoch(0)
        loader = DataLoader(ds, batch_size=args.batch_size, sampler=sampler,
                            shuffle=False)
        n_local = len(sampler)
    else:
        # Same sharding logic, but with no DDP wrapper and no sampler-based sync:
        # each rank independently walks its own slice, nothing is AllReduced.
        idx = list(range(rank, len(ds), world))
        n_local = len(idx)
        loader = DataLoader(torch.utils.data.Subset(ds, idx),
                            batch_size=args.batch_size, shuffle=False)

    opt = torch.optim.SGD(model.parameters(), lr=args.lr)
    loss_fn = nn.MSELoss()

    tag = f"[rank{rank}]"
    print(f"{tag} pid={os.getpid()} mode={args.mode} backend={backend or 'none'} "
          f"WORLD_SIZE={world} RANK={rank} LOCAL_RANK={local_rank} "
          f"torch={torch.__version__} cuda_available={torch.cuda.is_available()} "
          f"cpu_threads={torch.get_num_threads()}", flush=True)
    print(f"{tag} dataset_total={len(ds)} samples_on_this_rank={n_local} "
          f"steps={args.steps} batch_size={args.batch_size} "
          f"steps_per_epoch={len(loader)}", flush=True)

    t_start = time.perf_counter()
    step_times = []
    for step, (xb, yb) in enumerate(loader):
        if step >= args.steps:
            break
        t0 = time.perf_counter()
        pred = model(xb)
        loss = loss_fn(pred, yb)
        opt.zero_grad(set_to_none=True)
        loss.backward()                      # DDP hooks AllReduce grads right here

        # layer-0 gradient fingerprint (only meaningful when weights are in sync)
        g = model.module[0].weight.grad if args.mode == "ddp" else model[0].weight.grad
        gnorm = g.norm().item()
        ghead = [round(v, 6) for v in g[0][:3].tolist()]

        loss_mean = loss.detach().clone()
        if args.mode == "ddp":
            dist.all_reduce(loss_mean, op=dist.ReduceOp.AVG)   # same value on all ranks

        opt.step()
        dt = time.perf_counter() - t0
        step_times.append(dt)
        print(f"{tag} step={step} loss={loss.item():.6f} "
              f"loss_allreduce_mean={loss_mean.item():.6f} "
              f"grad_l2_layer0={gnorm:.6f} grad_head={ghead} "
              f"step_time_ms={dt * 1000:.1f}", flush=True)

    total = time.perf_counter() - t_start
    param_l2 = torch.cat([p.detach().reshape(-1) for p in
                          (model.module if args.mode == "ddp" else model).parameters()]).norm().item()
    print(f"{tag} SUMMARY mode={args.mode} rank={rank} world={world} "
          f"samples_on_this_rank={n_local} steps={len(step_times)} "
          f"grad_l2_layer0_last={gnorm:.6f} grad_head_last={ghead} "
          f"param_l2_after={param_l2:.6f} "
          f"train_seconds={total:.3f} mean_step_ms={1000 * sum(step_times) / len(step_times):.1f}",
          flush=True)

    if args.mode == "ddp":
        dist.barrier()
        dist.destroy_process_group()


if __name__ == "__main__":
    main()
