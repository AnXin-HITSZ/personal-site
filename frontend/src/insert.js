/* 在文本域的光标处插一段字，插完把光标摆到它后面。插图要用：图是传上去了，
   但插在哪一处、插完人接着往哪写，得由这一下说清楚。 */
export function insertAtCursor(area, text) {
  if (!area) return '';
  const start = area.selectionStart ?? area.value.length;
  const end = area.selectionEnd ?? start;
  /* setRangeText 走的是浏览器自己的编辑命令，撤销栈还留着——Ctrl+Z 能把这一下
     撤掉。自己拼字符串再赋给 value，那一步就撤不回来了。 */
  area.setRangeText(text, start, end, 'end');
  area.focus();
  return area.value;
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
