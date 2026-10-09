/* 全站共用的一份 Markdown 渲染：文章详情页、编辑器预览、QA 页都从这里走，
   语法只在一处生长，三处不会慢慢变成三种。 */
import MarkdownIt from 'markdown-it';
import katex from 'katex';
/* KaTeX 的字形版式跟着这一份走，所以样式也在这一层引。 */
import 'katex/dist/katex.min.css';

/* html: false 是显式写下的：三处 v-html 的安全性都靠它——正文里手写的标签被转义成
   文字，能成为标签的只有 Markdown 语法自己生成的。 */
const md = new MarkdownIt({ html: false });

/* 公式写坏不挡整篇：原文照排、染印色。KaTeX 只吃十六进制，这个值对应 --seal。 */
const formula = { throwOnError: false, errorColor: '#a52a1e' };

/* $…$ 行内：不跨行，首尾不留空白——`$ 5 与 $ 6` 那种是钱，不是公式。 */
function mathInline(state, silent) {
  const start = state.pos;
  if (state.src[start] !== '$') return false;
  /* 紧挨着另一个 $ 的也不认：$$ 归独立成行那一档。 */
  if (state.src[start + 1] === '$' || state.src[start - 1] === '$') return false;
  let end = start + 1;
  for (; end < state.posMax; end++) {
    const ch = state.src[end];
    if (ch === '\\') { end++; continue; } /* 转义过的 $ 不算收口 */
    /* 反引号是代码段的地盘，扫到就收手，免得跨过界去咬里面的 $。 */
    if (ch === '\n' || ch === '`') return false;
    if (ch === '$') break;
  }
  if (end >= state.posMax) return false;
  /* 收口的 $ 后头跟着数字的也是钱：`$5 与 $6`、`定价 $5与$6 档`。 */
  if (/\d/.test(state.src.slice(end + 1, end + 2))) return false;
  const tex = state.src.slice(start + 1, end);
  if (/^\s|\s$/.test(tex)) return false;
  if (!silent) {
    const token = state.push('math_inline', 'span', 0);
    token.content = tex;
  }
  state.pos = end + 1;
  return true;
}

/* \(…\) 行内：与 $…$ 同一档，只是记号不同。也只在同一行里找收口。首尾留不留空白
   这边不管——$ 那边盯着的是钱，这边没有那种歧义。 */
function mathInlineParen(state, silent) {
  const start = state.pos;
  if (state.src[start] !== '\\' || state.src[start + 1] !== '(') return false;
  let end = start + 2;
  let closed = false;
  for (; end < state.posMax - 1; end++) {
    const ch = state.src[end];
    if (ch === '\\') {
      /* 收口就在眼前；\\（TeX 的换行记号）要整对跳过，它后面的 ) 不算收口。 */
      if (state.src[end + 1] === ')') { closed = true; break; }
      end++;
      continue;
    }
    /* 与 $ 那一支同理：不跨行、不进代码段的地盘。 */
    if (ch === '\n' || ch === '`') return false;
  }
  if (!closed) return false;
  const tex = state.src.slice(start + 2, end);
  if (!silent) {
    const token = state.push('math_inline', 'span', 0);
    token.content = tex;
  }
  state.pos = end + 2;
  return true;
}

/* 独立公式两族（$$…$$ 与 \[…\]）共用一套扫描，只有记号不同：都必须自己起一行，
   同一行收口，或从开记号那一行一直吃到收口那一行。收口记号后头还有字就不算收口
   ——宁可把那行当正文，也不悄悄吞掉半行。找不到收口就照原文排。 */
function mathBlockRule(open, close) {
  return function mathBlock(state, startLine, endLine, silent) {
    const start = state.bMarks[startLine] + state.tShift[startLine];
    if (state.src.slice(start, start + open.length) !== open) return false;
    const first = state.src.slice(start + open.length, state.eMarks[startLine]);
    let tex = null;
    let next = startLine;
    const sameLine = first.indexOf(close);
    if (sameLine >= 0 && !first.slice(sameLine + close.length).trim()) {
      tex = first.slice(0, sameLine);
    } else if (sameLine < 0) {
      const lines = [first];
      for (next = startLine + 1; next < endLine; next++) {
        const from = state.bMarks[next] + state.tShift[next];
        const line = state.src.slice(from, state.eMarks[next]);
        const at = line.indexOf(close);
        if (at >= 0 && !line.slice(at + close.length).trim()) {
          lines.push(line.slice(0, at));
          tex = lines.join('\n');
          break;
        }
        lines.push(line);
      }
    }
    if (tex === null) return false;
    if (!silent) {
      const token = state.push('math_block', 'div', 0);
      token.block = true;
      token.content = tex;
      token.map = [startLine, next + 1];
      state.line = next + 1;
    }
    return true;
  };
}

md.inline.ruler.before('escape', 'math_inline', mathInline);
/* 这一支必须排在 escape 前面：不抢在它前头，\( 会被它当成「转义过的左括号」，
   反斜杠当场就没了，KaTeX 再也见不到。 */
md.inline.ruler.before('escape', 'math_inline_paren', mathInlineParen);
const mathBlockAlt = { alt: ['paragraph', 'reference', 'blockquote', 'list'] };
md.block.ruler.before('fence', 'math_block', mathBlockRule('$$', '$$'), mathBlockAlt);
md.block.ruler.before('fence', 'math_block_bracket', mathBlockRule('\\[', '\\]'), mathBlockAlt);
md.renderer.rules.math_inline = (tokens, idx) =>
  katex.renderToString(tokens[idx].content, { ...formula, displayMode: false });
/* 独立公式套一层 .katex-block：居中与横滚归它管（见 styles.css 的正文一节）。 */
md.renderer.rules.math_block = (tokens, idx) =>
  '<div class="katex-block">' + katex.renderToString(tokens[idx].content, { ...formula, displayMode: true }) + '</div>\n';

export function renderMarkdown(source) {
  return md.render(source);
}
