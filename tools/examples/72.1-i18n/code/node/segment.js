// 实测 E3/E4（Node/ICU 侧）：Intl.Segmenter 数 grapheme，并与 Go uniseg 对比；
// 泰语用 ICU 词典分词，证明「无空格语言没有分词器就断不了行」。
const fs = require('fs');
const path = require('path');

const samples = {
  'ascii-a': 'a',
  'e-combining': 'e\u0301',
  'flag-cn': '🇨🇳',
  'zwj-family': '👨‍👩‍👧‍👦',
  'skin-tone': '👍🏽',
  'zwj-profession': '👩‍💻',
  'keycap-1': '1️⃣',
  'heart-vs16': '❤️',
};
const seg = new Intl.Segmenter('en', { granularity: 'grapheme' });
const gc = s => [...seg.segment(s)].length;
console.log(`node=${process.version} icu=${process.versions.icu}`);

const go = new Map();
for (const line of fs.readFileSync(path.join(__dirname, '..', '..', 'raw', 'e-grapheme-go.tsv'), 'utf8').trim().split('\n').slice(1)) {
  const f = line.split('\t');
  go.set(f[0], { bytes: +f[1], runes: +f[2], graphemes: +f[3] });
}
console.log('case\tbytes\trunes\tgo_graphemes\ticu_graphemes\tmatch');
let same = 0;
for (const [k, s] of Object.entries(samples)) {
  const g = go.get(k);
  const icu = gc(s);
  const ok = g.graphemes === icu;
  if (ok) same++;
  console.log(`${k}\t${Buffer.byteLength(s)}\t${[...s].length}\t${g.graphemes}\t${icu}\t${ok ? 'SAME' : 'DIFF'}`);
}
console.log(`## grapheme 一致 ${same}/${Object.keys(samples).length}`);

// 泰语：按空白切 1 段，ICU 词典分词 N 段
const thai = 'ภาษาไทยไม่มีช่องว่างระหว่างคำ';
const words = [...new Intl.Segmenter('th', { granularity: 'word' }).segment(thai)]
  .filter(s => s.isWordLike).map(s => s.segment);
console.log(`\n## 泰语分词 thai=${thai}`);
console.log(`whitespace_chunks=${thai.split(/\s+/).length} icu_wordlike=${words.length} segments=[${words.join('|')}]`);
