// 实测 B（Node/ICU 侧）：Intl.PluralRules 与 Go x/text 逐格对比。
// 运行环境：alpine:3.20 + nodejs 20.x（全程 ICU，非 small-icu）。
const fs = require('fs');
const path = require('path');

const langs = ['en', 'ru', 'ar', 'pl', 'zh', 'fr', 'de', 'ja', 'uk', 'cs'];
const nums = ['0', '1', '2', '3', '5', '11', '21', '100', '1.5'];
const valid = new Set(['zero', 'one', 'two', 'few', 'many', 'other']);

console.log(`node=${process.version} icu=${process.versions.icu} cldr=${process.versions.cldr}`);
console.log('lang\tn\tcategory');
for (const l of langs) {
  const pr = new Intl.PluralRules(l, { type: 'cardinal' });
  for (const n of nums) {
    const c = pr.select(Number(n));
    if (!valid.has(c)) throw new Error(`nonstandard category ${c}`);
    console.log(`${l}\t${n}\t${c}`);
  }
}

// ---- 读 Go 的 TSV，逐格对比 ----
const goTsv = path.join(__dirname, '..', '..', 'raw', 'b-plural-go.tsv');
const rows = fs.readFileSync(goTsv, 'utf8').trim().split('\n').slice(1);
const goMap = new Map();
for (const line of rows) {
  const f = line.split('\t');
  goMap.set(`${f[0]}|${f[1]}`, f[7]);
}
let same = 0, diff = 0;
const diffs = [];
for (const l of langs) {
  const pr = new Intl.PluralRules(l, { type: 'cardinal' });
  for (const n of nums) {
    const icu = pr.select(Number(n));
    const g = goMap.get(`${l}|${n}`);
    if (g === undefined) throw new Error(`missing Go row ${l}|${n}`);
    if (g === icu) same++; else { diff++; diffs.push(`${l} n=${n}: go=${g} icu=${icu}`); }
  }
}
console.log(`\n## 对比：${same + diff} 格，一致 ${same}，差异 ${diff}`);
if (diffs.length) console.log('## 差异明细：\n' + diffs.join('\n'));

// ---- 六类矩阵（给正文表格用）：语言 × 该类命中的 n ----
console.log('\n## 类别 -> 命中数字（Go 与 ICU 按 category 归并；两者一致时只列一列）');
const cats = ['zero', 'one', 'two', 'few', 'many', 'other'];
console.log('lang\t' + cats.join('\t'));
for (const l of langs) {
  const pr = new Intl.PluralRules(l, { type: 'cardinal' });
  const hit = Object.fromEntries(cats.map(c => [c, []]));
  for (const n of nums) hit[pr.select(Number(n))].push(n);
  console.log(l + '\t' + cats.map(c => hit[c].join(',') || '-').join('\t'));
}
