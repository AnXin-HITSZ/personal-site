const date = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit', timeZone: 'Asia/Shanghai',
});

export const formatDate = value => date.format(new Date(value)).replaceAll('/', '.');

const dateTime = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'Asia/Shanghai',
});

/* 设备列表要认出「这台是不是我刚才用的那台」，光有日期不够，得有钟点。 */
export const formatDateTime = value => dateTime.format(new Date(value)).replaceAll('/', '.');
