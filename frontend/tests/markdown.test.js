import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, rmSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/* src/markdown.js 顶上引着 KaTeX 的样式，node 读不了 .css。把那一行剥掉写成一份
   临时副本再引——解析规则与注册顺序一字不动，测的就是线上那份。 */
const temporary = fileURLToPath(new URL('./markdown-rules.tmp.mjs', import.meta.url));
const source = readFileSync(new URL('../src/markdown.js', import.meta.url), 'utf8')
  .replace(/^import 'katex\/dist\/katex\.min\.css';$/m, '');
assert.notEqual(source, readFileSync(new URL('../src/markdown.js', import.meta.url), 'utf8'),
  'KaTeX 样式那一行没剥到——markdown.js 的写法变了，这份用例要跟着改');
writeFileSync(temporary, source);

let renderMarkdown;
try {
  ({ renderMarkdown } = await import('./markdown-rules.tmp.mjs'));
} finally {
  rmSync(temporary, { force: true });
}

const isMath = html => /class="katex/.test(html);
const asText = html => html.replace(/<[^>]+>/g, '').trim();

test('$…$ 与 $$…$$ 两族照旧', () => {
  assert.ok(isMath(renderMarkdown('其值为 $a+b$ 时最大。')));
  assert.ok(isMath(renderMarkdown('$$\nF1=\\frac{2PR}{P+R}\n$$')));
});

/* \( 与 \[ 在 markdown-it 眼里是「转义过的括号」，escape 规则会把反斜杠吃掉。
   数学规则必须排在它前面，否则 KaTeX 连公式的影子都见不到。 */
test('反斜杠定界符与美元定界符同一待遇', () => {
  assert.ok(isMath(renderMarkdown('其值为 \\(a+b\\) 时最大。')));
  assert.ok(isMath(renderMarkdown('\\[\nF1=\\frac{2PR}{P+R}\n\\]')));
});

test('连续两个双反斜杠是 TeX 的换行记号，收口要认在它后面', () => {
  const html = renderMarkdown('\\(\\begin{matrix}a\\\\b\\end{matrix}\\)');
  assert.ok(isMath(html));
  assert.match(html, /a\\\\b/);
});

/* $$ 与 \[ 都只在自己起一行时才算独立公式——行中的写法和从前一样是正文。 */
test('独立公式要自己起一行', () => {
  assert.ok(!isMath(renderMarkdown('引一句 \\[a+b\\] 看看')));
  assert.ok(!isMath(renderMarkdown('引一句 $$a+b$$ 看看')));
  assert.equal(asText(renderMarkdown('引一句 \\[a+b\\] 看看')), '引一句 [a+b] 看看');
});

/* $ 那一支盯的是钱：$5 与 $6 不能被认成公式。 */
test('金额不是公式', () => {
  const html = renderMarkdown('价格是 $5 与 $6 两档。');
  assert.ok(!isMath(html));
  assert.equal(asText(html), '价格是 $5 与 $6 两档。');
});

test('代码段与围栏里的定界符原样留着', () => {
  const inline = renderMarkdown('行内代码：`$a+b$`、`\\(a+b\\)`。');
  assert.ok(!isMath(inline));
  assert.match(inline, /<code>\$a\+b\$<\/code>/);
  const fence = renderMarkdown('```text\n$a+b$\n$$a+b$$\n\\[a+b\\]\n```');
  assert.ok(!isMath(fence));
  assert.match(fence, /\$a\+b\$/);
  assert.match(fence, /\\\[a\+b\\\]/);
});

/* 没找到收口就照原文排：宁可留一对字面括号，也不能吞掉后面的正文。 */
test('不完整的定界符留在正文里，不吃后面的字', () => {
  const html = renderMarkdown('留下 \\(a+b 没闭合，后面还有字。');
  assert.ok(!isMath(html));
  assert.equal(asText(html), '留下 (a+b 没闭合，后面还有字。');
  const block = renderMarkdown('\\[\nF1=x\n没闭合');
  assert.ok(!isMath(block));
  assert.equal(asText(block).replace(/\s+/g, ''), '[F1=x没闭合');
});
