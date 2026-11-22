import os, torch, torch.distributed as dist, torch.nn as nn
dist.init_process_group(backend="gloo", init_method="env://")
local_rank = int(os.environ.get("LOCAL_RANK", 0))
model = nn.Linear(4, 2)
try:
    model = nn.parallel.DistributedDataParallel(model, device_ids=[local_rank])
    print(f"[rank{local_rank}] OK device_ids=[{local_rank}]")
except Exception as e:
    print(f"[rank{local_rank}] FAILED: {type(e).__name__}: {e}")
finally:
    dist.destroy_process_group()
