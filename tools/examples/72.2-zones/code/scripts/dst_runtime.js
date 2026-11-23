// gap/overlap 在 Node(V8) 中的默认行为
process.env.TZ = 'America/New_York';
const gap = new Date(2026, 2, 8, 2, 30, 0);
const ov = new Date(2026, 10, 1, 1, 30, 0);
const fmt = (d) => `${d.toString()} | ISO=${d.toISOString()} | offset=${-d.getTimezoneOffset()} min`;
console.log('gap     2026-03-08 02:30:', fmt(gap));
console.log('overlap 2026-11-01 01:30:', fmt(ov));
