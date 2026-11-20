import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { build } from 'esbuild';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const { outputFiles } = await build({
  stdin: { contents: "export { Gifts } from './src/pages/Gifts';", resolveDir: process.cwd() },
  bundle: true, platform: 'node', format: 'cjs', write: false, packages: 'external',
});
const compiled = { exports: {} };
new Function('require', 'module', 'exports', outputFiles[0].text)(createRequire(import.meta.url), compiled, compiled.exports);

test('gift management renders generation and audit controls without a selected account or character', () => {
  const html = renderToStaticMarkup(createElement(compiled.exports.Gifts, { notify() {} }));
  for (const text of ['公共口令', '活动编号', '物品 ID', '物品数量', '结束时间', '生成礼包码', '兑换记录']) assert.ok(html.includes(text), text);
  assert.match(html, /同活动每账号领取一次/);
  assert.match(html, /原码、原角色重试/);
  assert.match(html, /disabled=""[^>]*>生成礼包码/);
});
