/* 把一段字换成另一段。走的是浏览器自己的输入命令，于是这一下留在撤销栈里：Ctrl+Z
   撤得回来，而且整段只算一步——分三行插就是三下，撤一次只回去一行。
   execCommand 早就标了废弃，可它是唯一一条能把脚本改的字记进撤销栈的路：自己拼好
   字符串赋给 value，撤销栈整个作废；setRangeText 在 Chrome 里同样撤不回来，还会把
   之前打过的字一起从栈里抹掉（量过：先打一个字再用 setRangeText 插一个字符，撤销
   栈里一个字都不剩）。所以认它为主，不认它的浏览器退回 setRangeText——字照样改对，
   只是那一下撤不回来。 */
function replaceRange(area, from, to, text) {
  area.focus();
  area.setSelectionRange(from, to);
  const command = text === '' ? 'delete' : 'insertText';
  if (globalThis.document?.execCommand?.(command, false, text)) return area.value;
  area.setRangeText(text, from, to, 'end');
  return area.value;
}

/* 在文本域的光标处插一段字，插完把光标摆到它后面。插图要用：图是传上去了，
   但插在哪一处、插完人接着往哪写，得由这一下说清楚。 */
export function insertAtCursor(area, text) {
  if (!area) return '';
  const start = area.selectionStart ?? area.value.length;
  const end = area.selectionEnd ?? start;
  return replaceRange(area, start, end, text);
}

const indentUnit = '\t';

/* 一行一行地看正文：每一行的开头到结尾在整份正文里的位置。结尾不含那个换行符，
   所以空行的两端是同一个数。 */
function linesOf(value) {
  const lines = [];
  let start = 0;
  for (let i = 0; i <= value.length; i += 1) {
    if (i === value.length || value[i] === '\n') {
      lines.push({ start, end: i });
      start = i + 1;
    }
  }
  return lines;
}

/* 这个位置落在第几行：最后一个开头不越过它的那一行。 */
function lineIndex(lines, offset) {
  let index = 0;
  for (let i = 0; i < lines.length; i += 1) {
    if (lines[i].start > offset) break;
    index = i;
  }
  return index;
}

/* 该跟着动的那几行：从光标这一行到最后一行。末尾正好停在行首的那一行不算
   —— 它一个字都没被选上。 */
function touchedLines(value, start, end) {
  const lines = linesOf(value);
  return lines.slice(lineIndex(lines, start), lineIndex(lines, end > start ? end - 1 : end) + 1);
}

/* 一行退回多少：开头是制表符就退掉它；按空格缩进来的，退掉不超过四个。 */
function outdentWidth(text) {
  if (text.startsWith('\t')) return 1;
  let spaces = 0;
  while (spaces < 4 && text[spaces] === ' ') spaces += 1;
  return spaces;
}

/* 插入点落在光标左边、或者正好压着它的，都得把光标往后推一格。 */
function shiftForward(offset, at) {
  let front = 0;
  for (const position of at) if (offset >= position) front += indentUnit.length;
  return offset + front;
}

/* 删掉的几段里落在光标左边、或者把它包住的那部分，要从光标里扣掉。 */
function shiftBack(offset, cuts) {
  let back = 0;
  for (const cut of cuts) {
    if (offset >= cut.from + cut.width) back += cut.width;
    else if (offset > cut.from) back += offset - cut.from;
  }
  return offset - back;
}

/* 正文框里按 Tab 是缩进，不是跳到下一格。光标没选东西时就在光标处插一个制表符；
   选了东西，被选中的每一行都往前挪一格——空行不挪，缩进是给「这一行有东西」用的，
   往空行里塞一个制表符只留下一段看不见的空白。 */
export function indentLines(area) {
  if (!area) return '';
  const value = area.value;
  const start = area.selectionStart ?? 0;
  const end = area.selectionEnd ?? start;

  if (start === end) return replaceRange(area, start, start, indentUnit);

  const touched = touchedLines(value, start, end);
  const at = touched.filter(line => line.end > line.start).map(line => line.start);
  if (!at.length) return area.value;

  const from = at[0];
  const to = touched[touched.length - 1].end;
  const text = value.slice(from, to).split('\n')
    .map(line => (line === '' ? line : indentUnit + line))
    .join('\n');

  const changed = replaceRange(area, from, to, text);
  area.selectionStart = shiftForward(start, at);
  area.selectionEnd = shiftForward(end, at);
  area.focus();
  return changed;
}

/* 退回一格。光标没选东西时退的是光标所在的那一行——这一下和缩进不一样：缩进插在
   光标处，退回是把这一行开头那点空白拿掉，光标自己会跟着往左挪。 */
export function outdentLines(area) {
  if (!area) return '';
  const value = area.value;
  const start = area.selectionStart ?? 0;
  const end = area.selectionEnd ?? start;

  const touched = touchedLines(value, start, end);
  const widths = touched.map(line => outdentWidth(value.slice(line.start, line.end)));
  /* 这一行本来就没缩进：什么都不动。不是错误，也没什么可说的。 */
  if (!widths.some(width => width > 0)) return area.value;

  const from = touched[0].start;
  const to = touched[touched.length - 1].end;
  const text = value.slice(from, to).split('\n')
    .map((line, index) => line.slice(widths[index]))
    .join('\n');
  const cuts = touched.map((line, index) => ({ from: line.start, width: widths[index] }));

  const changed = replaceRange(area, from, to, text);
  area.selectionStart = shiftBack(start, cuts);
  area.selectionEnd = shiftBack(end, cuts);
  area.focus();
  return changed;
}

/* 这一格把 Tab 收作缩进用，可要是全占了，只用键盘的人就再也走不出去——WCAG 2.1.2
   说的键盘陷阱。所以留一条路：Esc 先把门打开，下一个 Tab 放它走；敲了别的键，门
   自己关上，不用记着「刚才按过 Esc 没有」。回两件事：门还开着没有，这一下该不该
   由我们接下（接下就是缩进，不接就让它照原样跳走）。 */
export function tabIntent(event, armed) {
  if (event.key === 'Escape') return { armed: true, indent: false };
  if (event.key !== 'Tab' || armed) return { armed: false, indent: false };
  return { armed: false, indent: true };
}

/* 文件名去掉扩展名当 alt：写图的人此刻手上只有这个名字，让他回头再补一句
   不如先给一个。 */
export function altFromFileName(name) {
  return String(name ?? '').replace(/\.[^.]+$/, '').trim();
}

/* 上传成功那一行要贴出刚插进去的 Markdown。整条 URL 有几十个字符，全写会把
   那一行撑到折行；掐头留尾，人认得出来是哪一张就够了。 */
export function shortenURL(url, keep = 26) {
  const value = String(url ?? '');
  if (value.length <= keep * 2 + 1) return value;
  return `${value.slice(0, keep)}…${value.slice(-keep)}`;
}

export function imageMarkdown(url, alt) {
  return `![${alt}](${url})`;
}
