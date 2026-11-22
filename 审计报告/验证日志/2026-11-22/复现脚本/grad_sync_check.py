import argparse, os, torch, torch.distributed as dist, torch.nn as nn
from torch.utils.data import DataLoader, Dataset, DistributedSampler

class DS(Dataset):
    def __init__(s, n=64, d=32, seed=1234):
        g = torch.Generator().manual_seed(seed)
        s.x = torch.randn(n, d, generator=g); w = torch.randn(d, 1, generator=g)
        s.y = s.x @ w
    def __len__(s): return len(s.x)
    def __getitem__(s, i): return s.x[i], s.y[i]

ap = argparse.ArgumentParser(); ap.add_argument("--mode", default="ddp"); a = ap.parse_args()
dist.init_process_group("gloo", init_method="env://")
rank, world = dist.get_rank(), dist.get_world_size()
torch.manual_seed(0)
net = nn.Sequential(nn.Linear(32, 64), nn.ReLU(), nn.Linear(64, 1))
if a.mode == "ddp":
    net = nn.parallel.DistributedDataParallel(net, device_ids=None)
ds = DS()
if a.mode == "ddp":
    sp = DistributedSampler(ds, num_replicas=world, rank=rank, shuffle=False, drop_last=False)
    ld = DataLoader(ds, batch_size=8, sampler=sp)
    inner = net.module
else:
    ld = DataLoader(torch.utils.data.Subset(ds, list(range(rank, len(ds), world))), batch_size=8)
    inner = net
xb, yb = next(iter(ld))
loss = nn.functional.mse_loss(net(xb), yb)
loss.backward()
g = inner[0].weight.grad.clone().reshape(-1)
buf = [torch.zeros_like(g) for _ in range(world)]
dist.all_gather(buf, g)
diffs = [ (g - o).abs().max().item() for o in buf ]
print(f"[rank{rank}] mode={a.mode} world={world} grad_l2={g.norm().item():.6f} "
      f"max_abs_grad_diff_vs_rank0={diffs[0]:.9g} vs_rank1={diffs[1]:.9g} max_over_all={max(diffs):.9g}", flush=True)
dist.barrier(); dist.destroy_process_group()
