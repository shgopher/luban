// 实测 D（Node/ICU 侧）：Intl.Collator 排序，与 Go x/text/collate 对比。
const fs = require('fs');
const path = require('path');
const words = ['Zebra', 'apple', 'Öl', 'Åsa', 'Ärger', 'a', 'z', 'éclair', 'Ödla', 'B', 'Arger', 'Asa'];
if (typeof process.versions.icu === 'undefined') throw new Error('no full ICU');
console.log(`node=${process.version} icu=${process.versions.icu} cldr=${process.versions.cldr}`);

console.log('## ICU 排序');
const orders = {};
for (const l of ['en', 'de', 'sv']) {
  const c = new Intl.Collator(l);
  const sorted = [...words].sort(c.compare);
  orders[l] = sorted;
  console.log(`${l}\t[${sorted.join(' ')}]`);
}
fs.writeFileSync(path.join(__dirname, '..', '..', 'raw', 'd-collate-node.tsv'),
  Object.entries(orders).map(([l, o]) => `${l}\t${o.join(' ')}`).join('\n') + '\n');

// 读 Go 结果比对顺序是否完全一致
const goLines = fs.readFileSync(path.join(__dirname, '..', '..', 'raw', 'd-collate-go.tsv'), 'utf8').trim().split('\n');
const go = {};
for (const line of goLines) {
  if (!/^(en|de|sv)\t\[/.test(line)) continue; // 跳过表头/注释/两两比较行
  const i = line.indexOf('\t');
  go[line.slice(0, i)] = line.slice(i + 1);
}
console.log('\n## Go vs ICU 顺序对比');
let ok = 0;
for (const l of ['en', 'de', 'sv']) {
  const g = go[l].replace(/[\[\]]/g, '');
  const same = g === orders[l].join(' ');
  if (same) ok++;
  console.log(`${l}\tGo=[${g}]\tICU=[${orders[l].join(' ')}]\t${same ? 'SAME' : 'DIFF'}`);
}
console.log(`一致 ${ok}/3`);
