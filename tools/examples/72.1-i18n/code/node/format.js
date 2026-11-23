// 实测 C（Node/ICU 侧）：数字 + 日期。日期只测「格式化样式/字段顺序」，
// 时区语义（同一时刻换时区换日）属于 72.2 的主场，这里固定 timeZone=UTC。
const fs = require('fs');
const path = require('path');

const locales = ['en-US', 'de-DE', 'fr-FR', 'en-IN', 'ar-EG', 'ja-JP'];
const values = [1234567.891, 0.5, -1234.5];
const cp = s => [...s].filter(ch => !/\p{Nd}/u.test(ch) && ch !== '-')
  .map(ch => 'U+' + ch.codePointAt(0).toString(16).toUpperCase().padStart(4, '0')).join(' ');
const firstDigit = s => {
  const d = [...s].find(ch => /\p{Nd}/u.test(ch));
  return d ? 'U+' + d.codePointAt(0).toString(16).toUpperCase().padStart(4, '0') : '-';
};

console.log(`node=${process.version} icu=${process.versions.icu} cldr=${process.versions.cldr} tz=${Intl.DateTimeFormat().resolvedOptions().timeZone}`);
console.log('## 数字（min/maxFractionDigits=3）');
console.log('locale\tvalue\tformatted\tnondigit_codepoints\tfirst_rune');
const out = [];
for (const l of locales) {
  const nf = new Intl.NumberFormat(l, { minimumFractionDigits: 3, maximumFractionDigits: 3 });
  for (const v of values) {
    const s = nf.format(v);
    out.push(`${l}\t${v}\t${s}\t${cp(s)}\t${firstDigit(s)}`);
    console.log(out[out.length - 1]);
  }
}
fs.writeFileSync(path.join(__dirname, '..', '..', 'raw', 'c-number-node.tsv'), 'locale\tvalue\tformatted\tnondigit_codepoints\tfirst_rune\n' + out.join('\n') + '\n');

// 与 Go 逐字节对比
const go = fs.readFileSync(path.join(__dirname, '..', '..', 'raw', 'c-number-go.tsv'), 'utf8').trim().split('\n').slice(1);
const gomap = new Map();
for (const line of go) { const f = line.split('\t'); gomap.set(`${f[0]}|${f[1]}`, { s: f[2], cp: f[3] }); }
let same = 0; const diffs = [];
for (const l of locales) {
  const nf = new Intl.NumberFormat(l, { minimumFractionDigits: 3, maximumFractionDigits: 3 });
  for (const v of values) {
    const s = nf.format(v), g = gomap.get(`${l}|${v}`);
    if (g && g.s === s) same++; else diffs.push(`${l} ${v}: go="${g && g.s}" icu="${s}"`);
  }
}
console.log(`\n## 数字 Go vs ICU：${same + diffs.length} 格，逐字节一致 ${same}，差异 ${diffs.length}`);
console.log(diffs.join('\n'));

console.log('\n## 日期：同一时刻 2026-11-21T18:30:00Z，timeZone=UTC');
console.log('locale\tshort\tlong\t字段顺序(numeric)');
const d = new Date('2026-11-21T18:30:00Z');
for (const l of locales) {
  const short = new Intl.DateTimeFormat(l, { dateStyle: 'short', timeZone: 'UTC' }).format(d);
  const long = new Intl.DateTimeFormat(l, { dateStyle: 'long', timeZone: 'UTC' }).format(d);
  // 用 numeric 字段单独拼，看年/月/日的排列；2-digit 保证可辨认
  const y = new Intl.DateTimeFormat(l, { year: 'numeric', timeZone: 'UTC' }).format(d);
  const m = new Intl.DateTimeFormat(l, { month: 'numeric', timeZone: 'UTC' }).format(d);
  const day = new Intl.DateTimeFormat(l, { day: 'numeric', timeZone: 'UTC' }).format(d);
  const order = new Intl.DateTimeFormat(l, { year: 'numeric', month: 'numeric', day: 'numeric', timeZone: 'UTC' }).format(d);
  console.log(`${l}\t${short}\t${long}\t${order}  (y=${y},m=${m},d=${day})`);
}

// 数字系统：同一数字在各 locale 下的默认数字系统
console.log('\n## 默认数字系统（Intl.NumberFormat 输出 1234）');
for (const l of locales) console.log(`${l}\t${new Intl.NumberFormat(l, { useGrouping: false }).format(1234)}`);
