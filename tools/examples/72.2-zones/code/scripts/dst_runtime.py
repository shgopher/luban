# gap/overlap 在 Python zoneinfo 中的默认行为（fold 参数）
from datetime import datetime, timezone
from zoneinfo import ZoneInfo

ny = ZoneInfo("America/New_York")
for fold in (0, 1):
    d = datetime(2026, 3, 8, 2, 30, tzinfo=ny, fold=fold)
    print(f"gap  2026-03-08 02:30 fold={fold}: wall={d.isoformat()} utc={d.astimezone(timezone.utc).isoformat()}")
for fold in (0, 1):
    d = datetime(2026, 11, 1, 1, 30, tzinfo=ny, fold=fold)
    print(f"overlap 2026-11-01 01:30 fold={fold}: wall={d.isoformat()} utc={d.astimezone(timezone.utc).isoformat()}")
