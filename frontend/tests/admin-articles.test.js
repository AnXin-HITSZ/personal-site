import test from 'node:test';
import assert from 'node:assert/strict';
import {
  articleLimits, articleProblems, createArticle, deleteArticle, getAdminArticle,
  listAdminArticles, statusText, updateArticle, uploadImage,
} from '../src/api/admin-articles.js';
import { altFromFileName, imageMarkdown, indentLines, insertAtCursor, outdentLines, shortenURL, tabIntent } from '../src/insert.js';
import { uploadText } from '../src/copy.js';
import { codes } from '../src/api/client.js';

function json(body, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

function capture(result, status = 200) {
  const seen = {};
  return {
    seen,
    fetchImpl: async (url, options) => {
      Object.assign(seen, { url, options });
      return result === null ? new Response(null, { status }) : json(result, status);
    },
  };
}

/* 一份形状正确的摘要，各条测试在它上面改一处再断言被拒。 */
function summary(overrides = {}) {
  return {
    id: 'abcd2345',
    slug: 'go-api-first-step',
    title: '从零写一个 Go API',
    summary: '把路由、中间件和错误处理串成一条线。',
    category: 'backend',
    tags: ['go', 'api'],
    status: 'published',
    publishedAt: '2026-09-20T02:00:00Z',
    readingMinutes: 6,
    createdAt: '2026-09-19T02:00:00Z',
    updatedAt: '2026-09-20T02:00:00Z',
    ...overrides,
  };
}

const detail = (overrides = {}) => ({ ...summary(), body: '# 标题\n\n正文', ...overrides });

const listBody = (query, overrides = {}) => ({
  items: [summary()],
  pagination: { page: query.page, pageSize: query.pageSize, total: 1, totalPages: 1 },
  ...overrides,
});

/* 分类的选项不在这儿：它是一份随写作变的数据，由 /admin/categories 现取
   （见 tests/categories.test.js）。这里只剩状态那两个词。 */
test('the writing desk names the two statuses the way the site does', () => {
  assert.deepEqual(statusText, { draft: '草稿', published: '已发布' });
});

test('local validation names one field at a time and agrees with the server limits', () => {
  const good = { slug: 'go-api', title: '标题', summary: '摘要', body: '正文' };
  assert.deepEqual(articleProblems(good), { slug: '', title: '', summary: '', body: '' });

  assert.match(articleProblems({ ...good, slug: 'Go API' }).slug, /小写字母/);
  assert.match(articleProblems({ ...good, slug: '-go' }).slug, /小写字母/);
  assert.match(articleProblems({ ...good, slug: '' }).slug, /小写字母/);
  assert.match(articleProblems({ ...good, title: '   ' }).title, /标题/);
  assert.match(articleProblems({ ...good, summary: '' }).summary, /摘要/);
  assert.match(articleProblems({ ...good, body: ' ' }).body, /正文/);

  // 按字符数（rune）量，不按字节：一个汉字就是 1。
  assert.equal(articleProblems({ ...good, title: '字'.repeat(articleLimits.titleRunes) }).title, '');
  assert.match(articleProblems({ ...good, title: '字'.repeat(articleLimits.titleRunes + 1) }).title, /255/);
  assert.equal(articleProblems({ ...good, summary: '字'.repeat(articleLimits.summaryRunes) }).summary, '');
  assert.match(articleProblems({ ...good, summary: '字'.repeat(articleLimits.summaryRunes + 1) }).summary, /500/);
  assert.equal(articleProblems({ ...good, body: '字'.repeat(articleLimits.bodyRunes) }).body, '');
  assert.match(articleProblems({ ...good, body: '字'.repeat(articleLimits.bodyRunes + 1) }).body, /200000/);
});

test('the list request omits the filters that mean "everything"', async () => {
  const query = { page: 1, pageSize: 10 };
  const { seen, fetchImpl } = capture(listBody(query));
  const result = await listAdminArticles(query, { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/articles?page=1&pageSize=10');
  assert.equal(seen.options.method, 'GET');
  assert.equal(seen.options.headers['X-Requested-With'], undefined);
  assert.equal(result.items.length, 1);
});

test('draft and category filters are carried, and a keyword is encoded', async () => {
  const query = { page: 2, pageSize: 10, status: 'draft', category: 'notes', q: '读书 笔记' };
  const { seen, fetchImpl } = capture({ items: [], pagination: { page: 2, pageSize: 10, total: 10, totalPages: 1 } });
  await listAdminArticles(query, { fetchImpl });

  // URLSearchParams 把空格写成 +；Gin 读回来是同一个空格。
  assert.equal(
    seen.url,
    `/api/v1/admin/articles?page=2&pageSize=10&status=draft&category=notes&q=${encodeURIComponent('读书 笔记').replace('%20', '+')}`,
  );
});

/* 草稿没有发布时刻，这是正常数据，不是违规——公开接口那一族 DTO 在这里不能套用。 */
test('a draft with a null publishedAt is a valid row', async () => {
  const query = { page: 1, pageSize: 10 };
  const draft = summary({ status: 'draft', publishedAt: null });
  const result = await listAdminArticles(query, { fetchImpl: async () => json(listBody(query, { items: [draft] })) });
  assert.equal(result.items[0].status, 'draft');
});

/* 形状对了就够了：某个分类还在不在由服务端说话（外键才拦得住），前端分不出来，也不
   该分——它要是拿一份自己的名单去卡，那份名单迟早和库里那一份对不上。以前这里有一
   条「all 不是分类」，现在也归到这一条上：all 的形状合法，写不写得进去由外键说。 */
test('a well-shaped category this client has never heard of is still a valid row', async () => {
  const query = { page: 1, pageSize: 10 };
  const result = await listAdminArticles(query, {
    fetchImpl: async () => json(listBody(query, { items: [summary({ category: 'zzzz9999' })] })),
  });
  assert.equal(result.items[0].category, 'zzzz9999');
});

test('a row whose shape drifted is rejected rather than rendered', async () => {
  const query = { page: 1, pageSize: 10 };
  const broken = [
    summary({ publishedAt: null }),                       // 已发布却没有发布时刻
    summary({ status: 'draft' }),                         // 草稿却带着发布时刻
    summary({ category: 'Backend' }),                     // 分类 id 只有小写
    summary({ category: 'backend!' }),
    summary({ tags: 'go' }),                              // 标签必须是数组
    summary({ readingMinutes: 0 }),                       // 时长下限是 1
    summary({ id: 'ABCD2345' }),                          // id 只有小写
    summary({ id: 'short' }),
    summary({ createdAt: 'invalid' }),
  ];
  for (const row of broken) {
    const cause = await listAdminArticles(query, {
      fetchImpl: async () => json(listBody(query, { items: [row] })),
    }).catch(error => error);
    assert.match(cause.message, /数据格式/, `${JSON.stringify(row)} 应当被拒`);
  }
});

test('pagination has to agree with the rows that came with it', async () => {
  const query = { page: 1, pageSize: 10 };
  const cases = [
    { items: [summary()], pagination: { page: 2, pageSize: 10, total: 1, totalPages: 1 } },
    { items: [summary()], pagination: { page: 1, pageSize: 20, total: 1, totalPages: 1 } },
    { items: [summary()], pagination: { page: 1, pageSize: 10, total: 1, totalPages: 3 } },
    { items: [], pagination: { page: 1, pageSize: 10, total: 1, totalPages: 1 } },
    { pagination: { page: 1, pageSize: 10, total: 1, totalPages: 1 } },
  ];
  for (const body of cases) {
    await assert.rejects(listAdminArticles(query, { fetchImpl: async () => json(body) }), /数据格式/);
  }
});

test('the detail read checks the id it asked for', async () => {
  const { seen, fetchImpl } = capture(detail());
  const article = await getAdminArticle('abcd2345', { fetchImpl });
  assert.equal(seen.url, '/api/v1/admin/articles/abcd2345');
  assert.equal(article.body, '# 标题\n\n正文');

  const other = await getAdminArticle('abcd2345', { fetchImpl: async () => json(detail({ id: 'zzzz9999' })) })
    .catch(error => error);
  assert.match(other.message, /数据格式/);
});

/* bodyRunes 还不在后端。缺了它只该少显示一段字，不该把整篇判成坏的；
   但它一旦出现，形状就得对——半个数字比没有数字更糟。 */
test('bodyRunes is tolerated while absent and checked once it appears', async () => {
  const tolerated = await getAdminArticle('abcd2345', { fetchImpl: async () => json(detail()) });
  assert.equal(tolerated.bodyRunes, null);

  const present = await getAdminArticle('abcd2345', { fetchImpl: async () => json(detail({ bodyRunes: 2140 })) });
  assert.equal(present.bodyRunes, 2140);

  for (const value of [1.5, -1, '2140', null]) {
    await assert.rejects(
      getAdminArticle('abcd2345', { fetchImpl: async () => json(detail({ bodyRunes: value })) }),
      /数据格式/,
    );
  }
});

test('create posts the whole draft and trims what the server will trim', async () => {
  const input = {
    slug: '  go-api  ', title: '  标题  ', summary: '  摘要  ', body: '正文',
    category: 'backend', tags: ['go'], status: 'draft',
  };
  const { seen, fetchImpl } = capture(detail({ status: 'draft', publishedAt: null, slug: 'go-api' }));
  await createArticle(input, { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/articles');
  assert.equal(seen.options.method, 'POST');
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  assert.equal(seen.options.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(seen.options.body), {
    slug: 'go-api', title: '标题', summary: '摘要', body: '正文',
    category: 'backend', tags: ['go'], status: 'draft',
  });
});

test('update targets the id and a missing article comes back as an error', async () => {
  const { seen, fetchImpl } = capture(detail());
  await updateArticle('abcd2345', { slug: 'go-api', title: 't', summary: 's', body: 'b', category: 'ai', tags: [], status: 'published' }, { fetchImpl });
  assert.equal(seen.url, '/api/v1/admin/articles/abcd2345');
  assert.equal(seen.options.method, 'PUT');

  const cause = await updateArticle('zzzz9999', {}, {
    fetchImpl: async () => json({ error: { code: 'NOT_FOUND', message: '文章不存在' } }, 404),
  }).catch(error => error);
  assert.equal(cause.code, codes.notFound);
});

test('a taken slug arrives as CONFLICT and points at the slug field', async () => {
  const cause = await createArticle({}, {
    fetchImpl: async () => json({
      error: { code: 'CONFLICT', message: '这个 slug 已经被别的文章用了', field: 'slug' },
    }, 409),
  }).catch(error => error);

  assert.equal(cause.code, codes.conflict);
  assert.equal(cause.field, 'slug');
});

test('delete treats 204 as success and encodes the id', async () => {
  const { seen, fetchImpl } = capture(null, 204);
  assert.equal(await deleteArticle('abcd2345', { fetchImpl }), null);
  assert.equal(seen.url, '/api/v1/admin/articles/abcd2345');
  assert.equal(seen.options.method, 'DELETE');
});

test('the upload posts a form and never sets its own Content-Type', async () => {
  const uploaded = { url: 'https://cdn.example.com/articles/ab/abcd.png', key: 'articles/ab/abcd.png', bytes: 1024, contentType: 'image/png' };
  const { seen, fetchImpl } = capture(uploaded, 201);
  const result = await uploadImage(new Blob([new Uint8Array([1])], { type: 'image/png' }), { fetchImpl });

  assert.equal(seen.url, '/api/v1/admin/uploads');
  assert.equal(seen.options.method, 'POST');
  assert.ok(seen.options.body instanceof FormData);
  assert.equal(seen.options.body.get('file').type, 'image/png');
  // 写死 boundary 会让服务端解不出表单，所以这个头必须不在。
  assert.equal(seen.options.headers['Content-Type'], undefined);
  assert.equal(seen.options.headers['X-Requested-With'], 'XMLHttpRequest');
  assert.equal(result.key, 'articles/ab/abcd.png');
});

test('a malformed upload response is rejected', async () => {
  const cases = [
    { url: 'x', key: 'y', bytes: 0, contentType: 'image/png' },
    { url: 'x', key: 'y', bytes: 1.5, contentType: 'image/png' },
    { url: 'x', key: 'y', bytes: 1, contentType: 7 },
    { url: 'x', key: 'y', bytes: 1 },
  ];
  for (const body of cases) {
    await assert.rejects(uploadImage(new Blob([]), { fetchImpl: async () => json(body, 201) }), /数据格式/);
  }
});

test('the upload failure wording says what to do next', () => {
  assert.match(uploadText({ code: codes.payloadTooLarge, offline: false }), /4 MB/);
  assert.match(uploadText({ code: codes.invalidArgument, offline: false }), /SVG/);
  assert.match(uploadText({ code: codes.serviceUnavailable, offline: false }), /OSS_/);
  assert.match(uploadText({ code: codes.internal, offline: false }), /服务暂时不可用/);
  assert.match(uploadText({ offline: true }), /连接不上服务器/);
});

/* 文本域要用真的：这些函数拿到的就是真元素上的选区与值，替身只是照着拼字符串。
   撤销栈那件事替身管不了——它由浏览器管，所以另有一条测试把 execCommand 替出来，
   钉住「改动走的是那条命令」。 */
function fakeArea(value, start = value.length, end = start) {
  return {
    value, selectionStart: start, selectionEnd: end, focused: false,
    setRangeText(text, from, to) {
      this.value = this.value.slice(0, from) + text + this.value.slice(to);
      this.selectionStart = this.selectionEnd = from + text.length;
    },
    setSelectionRange(from, to) { this.selectionStart = from; this.selectionEnd = to; },
    focus() { this.focused = true; },
  };
}

/* 浏览器那条输入命令的替身：把选中的一段换掉，光标落到新字后面（真浏览器就是这么
   做的，此外它还会把这一笔记进撤销栈——那一步替身复现不了，也不需要复现）。 */
function fakeCommand(area, calls) {
  return {
    execCommand(command, _ui, text) {
      calls.push({ command, text });
      const { selectionStart: from, selectionEnd: to } = area;
      const inserted = command === 'insertText' ? text : '';
      area.value = area.value.slice(0, from) + inserted + area.value.slice(to);
      area.selectionStart = area.selectionEnd = from + inserted.length;
      return true;
    },
  };
}

test('an image lands at the cursor and leaves it after the markdown', () => {
  const area = fakeArea('开头结尾', 2, 2);
  assert.equal(insertAtCursor(area, '![图](u)'), '开头![图](u)结尾');
  assert.equal(area.selectionStart, 2 + '![图](u)'.length);
  assert.equal(area.focused, true);
});

test('a selection is replaced, and a missing textarea is not a crash', () => {
  assert.equal(insertAtCursor(fakeArea('abcdef', 1, 4), 'X'), 'aXef');
  assert.equal(insertAtCursor(fakeArea('尾巴'), 'X'), '尾巴X');
  assert.equal(insertAtCursor(null, 'X'), '');
});

/* Tab 那几条也拿真的文本域：它们改字走的是那条浏览器命令，理由和插图一样。 */

test('a tab lands at the cursor when nothing is selected', () => {
  const area = fakeArea('ab', 1, 1);
  assert.equal(indentLines(area), 'a\tb');
  assert.equal(area.selectionStart, 2);
  assert.equal(area.focused, true);
  assert.equal(indentLines(fakeArea('', 0, 0)), '\t');
  assert.equal(indentLines(null), '');
});

test('every line the selection touches moves over by one tab', () => {
  const area = fakeArea('aa\nbb\ncc', 1, 7);
  assert.equal(indentLines(area), '\taa\n\tbb\n\tcc');
  // 选中的那一段字还是原来那一段：两头跟着挪，第二行之后插进去的制表符都落在框里。
  assert.equal(area.value.slice(area.selectionStart, area.selectionEnd), 'a\n\tbb\n\tc');
});

test('a selection that stops at a line start does not move that line', () => {
  /* 末尾正好停在行首，说明最后那一行一个字都没被选上。 */
  const area = fakeArea('aa\nbb', 0, 3);
  assert.equal(indentLines(area), '\taa\nbb');
});

test('an empty line is left alone', () => {
  /* 缩进是给「这一行有东西」用的。往空行里塞一个制表符，只留下一段看不见的空白。 */
  const area = fakeArea('aa\n\nbb', 0, 6);
  assert.equal(indentLines(area), '\taa\n\n\tbb');
});

test('shift-tab takes back one tab, or up to four spaces', () => {
  const tabs = fakeArea('\taa\n\t\tbb', 0, 8);
  assert.equal(outdentLines(tabs), 'aa\n\tbb');
  assert.equal(tabs.value.slice(tabs.selectionStart, tabs.selectionEnd), 'aa\n\tbb');

  /* 第二行按两个空格缩进来，也退得掉——一次只退一格，多的留给下一次。 */
  const spaces = fakeArea('    aa\nbb\n  cc', 0, 14);
  assert.equal(outdentLines(spaces), 'aa\nbb\ncc');
});

test('shift-tab works on the line the cursor is on, and gives up quietly', () => {
  const area = fakeArea('    aa', 6, 6);
  assert.equal(outdentLines(area), 'aa');
  assert.equal(area.selectionStart, 2);

  // 这一行本来就没缩进，那就什么都不动——不是错误，也没什么可说的。
  const flat = fakeArea('aa\nbb', 4, 4);
  assert.equal(outdentLines(flat), 'aa\nbb');
  assert.equal(flat.selectionStart, 4);
  assert.equal(outdentLines(null), '');
});

/* 撤销栈是浏览器的事，替身测不出来，但「改动有没有走那条命令」测得出——只有走它，
   那一下才留得进撤销栈，而且一整段只下一次命令，撤销栈里就只留一步。 */
test('an edit goes through the browser command, once for the whole span', () => {
  const calls = [];
  const area = fakeArea('aa\nbb');
  area.setSelectionRange(0, 5);
  globalThis.document = fakeCommand(area, calls);
  try {
    assert.equal(indentLines(area), '\taa\n\tbb');
    assert.deepEqual(calls, [{ command: 'insertText', text: '\taa\n\tbb' }]);
    /* 选中的还是原来那一段字：第一行那个制表符插在选框前头，落在框外。 */
    assert.equal(area.value.slice(area.selectionStart, area.selectionEnd), 'aa\n\tbb');
  } finally {
    delete globalThis.document;
  }
});

test('taking back a tab replaces the span, and a line of blanks is deleted outright', () => {
  const calls = [];
  const area = fakeArea('\taa\n\tbb', 0, 8);
  globalThis.document = fakeCommand(area, calls);
  try {
    assert.equal(outdentLines(area), 'aa\nbb');
    assert.deepEqual(calls, [{ command: 'insertText', text: 'aa\nbb' }]);
  } finally {
    delete globalThis.document;
  }

  /* 这一行整行都是空白：新的一段是空的，那就没有「插」这件事，只能删。 */
  const blanks = [];
  const blank = fakeArea('aa\n  \nbb', 3, 6);
  globalThis.document = fakeCommand(blank, blanks);
  try {
    assert.equal(outdentLines(blank), 'aa\n\nbb');
    assert.deepEqual(blanks, [{ command: 'delete', text: '' }]);
  } finally {
    delete globalThis.document;
  }
});

/* 出路那条路：这一格把 Tab 收走了，就得留一个只用键盘也走得出去的办法。 */
test('escape opens the way out of the body box, and the next tab takes it', () => {
  /* 平常的样子：Tab 归缩进。 */
  assert.deepEqual(tabIntent({ key: 'Tab' }, false), { armed: false, indent: true });
  assert.deepEqual(tabIntent({ key: 'Tab', shiftKey: true }, false), { armed: false, indent: true });

  /* Esc 打开门，下一个 Tab 放走；门用过一次就自己关上。 */
  assert.deepEqual(tabIntent({ key: 'Escape' }, false), { armed: true, indent: false });
  assert.deepEqual(tabIntent({ key: 'Tab' }, true), { armed: false, indent: false });
  assert.deepEqual(tabIntent({ key: 'Tab', shiftKey: true }, true), { armed: false, indent: false });

  /* 门开着，可中间敲了别的键：收回来，Tab 又归缩进。 */
  assert.deepEqual(tabIntent({ key: 'a' }, true), { armed: false, indent: false });
  assert.deepEqual(tabIntent({ key: 'Tab' }, false), { armed: false, indent: true });
});

test('alt text comes from the file name and a long url is shortened', () => {
  assert.equal(altFromFileName('屏幕截图 2026-09-28.png'), '屏幕截图 2026-09-28');
  assert.equal(altFromFileName('no-extension'), 'no-extension');
  assert.equal(altFromFileName(''), '');
  assert.equal(imageMarkdown('u', 'a'), '![a](u)');

  const url = `https://cdn.example.com/${'a'.repeat(120)}.png`;
  const short = shortenURL(url);
  assert.ok(short.length < url.length);
  assert.match(short, /…/);
  assert.equal(shortenURL('https://cdn.example.com/a.png'), 'https://cdn.example.com/a.png');
});
