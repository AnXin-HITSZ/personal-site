/* 时区写死 Asia/Shanghai：库里存的是 UTC，而这一站只有一种「哪一天」——作者所在的那一天。
   跟着读者机器的时区走的话，同一篇在不同地方会显示成不同的日期。 */
const date = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit', timeZone: 'Asia/Shanghai',
});

// 斜杠换成点：两条斜杠在版面上太吵，点安静，也不至于被看成路径。
export const formatDate = value => date.format(new Date(value)).replaceAll('/', '.');

const dateTime = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'Asia/Shanghai',
});

/* 设备列表要认出「这台是不是我刚才用的那台」，光有日期不够，得有钟点。 */
export const formatDateTime = value => dateTime.format(new Date(value)).replaceAll('/', '.');

/* 写作台那一栏里的日期。「改于 9 月 28 日」比「2026.09.28」少占一截，也更像一句话
   ——那一栏本来就是在说「改于」。年份另起一行写着，所以这一格不要年份。

   拼出来而不是让 Intl 排版：zh-CN 只给月日时排的是「9/28」，不是「9月28日」——
   那是数值写法，读起来是另一回事。 */
const monthDay = new Intl.DateTimeFormat('zh-CN', {
  month: 'numeric', day: 'numeric', timeZone: 'Asia/Shanghai',
});

export function formatMonthDay(value) {
  const parts = monthDay.formatToParts(new Date(value));
  const part = type => parts.find(item => item.type === type).value;
  return `${part('month')} 月 ${part('day')} 日`;
}
