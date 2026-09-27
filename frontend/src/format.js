const date = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit', timeZone: 'Asia/Shanghai',
});

export const formatDate = value => date.format(new Date(value)).replaceAll('/', '.');
