import { createApp } from '../server.mjs';
import { createFixtureFetch, TEST_CREDENTIAL } from './fixtures.mjs';

const fixture = createFixtureFetch();
const server = createApp({ fetchImpl: fixture.fetchImpl });
server.listen(3001, '127.0.0.1', () => {
  console.log(`仅用于浏览器测试的模拟百度接口，不连接真实账号。\nhttp://127.0.0.1:3001\n测试 BDUSS: ${TEST_CREDENTIAL}`);
});
