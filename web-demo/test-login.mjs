const RSA_MODULUS = BigInt('0xB3C61EBBA4659C4CE3639287EE871F1F48F7930EA977991C7AFE3CC442FEA49643212E7D570C853F368065CC57A2014666DA8AE7D493FD47D171C0D894EEE3ED7F99F6798B7FFD7B5873227038AD23E3197631A8CB642213B9F27D4901AB0D92BFA27542AE890855396ED92775255C977F5C302F1E7ED4B1E369C12CB6B1822F');
const RSA_EXPONENT = BigInt(0x10001);

function modPow(base, exp, mod) {
  let result = 1n;
  base = base % mod;
  while (exp > 0n) {
    if (exp & 1n) result = (result * base) % mod;
    exp >>= 1n;
    base = (base * base) % mod;
  }
  return result;
}

function rsaEncryptHex(plaintext, preEncode) {
  const data = preEncode ? Buffer.from(plaintext).toString('base64') : plaintext;
  const value = BigInt('0x' + Buffer.from(data).toString('hex'));
  return modPow(value, RSA_EXPONENT, RSA_MODULUS).toString(16).padStart(256, '0');
}

const username = process.argv[2];
const password = process.argv[3];
const encUser = rsaEncryptHex(username, false);
const encPwd = rsaEncryptHex(password, true);
const ts = String(Date.now());

// Test A: /v2/api/login with redirect: 'manual' to see Location header
async function testWebLoginRedirect() {
  console.log('--- Test A: /v2/api/login (check redirect location) ---');
  const params = new URLSearchParams({
    username: encUser,
    password: encPwd,
    isphone: '0',
    isEncrypted: '1',
    staticpage: 'https://pan.baidu.com/res/static/thirdparty/loginResult.html',
    charset: 'UTF-8',
    token: '',
    tpl: 'netdisk',
    apiver: 'v3',
    tt: ts,
    u: 'https://pan.baidu.com/',
    papi: '1',
  });
  const response = await fetch('https://passport.baidu.com/v2/api/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
      Referer: 'https://pan.baidu.com/',
      Origin: 'https://pan.baidu.com',
    },
    body: params,
    redirect: 'manual',
  });
  console.log('Status:', response.status);
  console.log('Headers:');
  for (const [key, value] of response.headers) {
    console.log(`  ${key}: ${value}`);
  }
  const text = await response.text();
  console.log('Body:', text.slice(0, 1000));
}

// Test B: Get a proper token first
async function testGetToken() {
  console.log('\n--- Test B: Get proper login token ---');
  const url = new URL('https://passport.baidu.com/v2/api/?getapi');
  url.searchParams.set('tpl', 'netdisk');
  url.searchParams.set('apiver', 'v3');
  url.searchParams.set('tt', ts);
  url.searchParams.set('class', 'login');
  url.searchParams.set('logintype', 'dialogLogin');
  url.searchParams.set('callback', 'callback');

  const response = await fetch(url.href, {
    headers: {
      'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
      Referer: 'https://pan.baidu.com/',
    },
  });
  const text = await response.text();
  console.log('Raw response:', text.slice(0, 800));

  // Extract token
  const tokenMatch = text.match(/"token"\s*:\s*"([^"]*)"/);
  if (tokenMatch) {
    console.log('\nExtracted token:', tokenMatch[1]);

    // Now try login with this token
    console.log('\n--- Test B2: Login with token ---');
    const params = new URLSearchParams({
      username: encUser,
      password: encPwd,
      isphone: '0',
      isEncrypted: '1',
      staticpage: 'https://pan.baidu.com/res/static/thirdparty/loginResult.html',
      charset: 'UTF-8',
      token: tokenMatch[1],
      tpl: 'netdisk',
      apiver: 'v3',
      tt: String(Date.now()),
      u: 'https://pan.baidu.com/',
      papi: '1',
    });
    const loginResp = await fetch('https://passport.baidu.com/v2/api/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
        Referer: 'https://pan.baidu.com/',
        Origin: 'https://pan.baidu.com',
      },
      body: params,
      redirect: 'manual',
    });
    console.log('Status:', loginResp.status);
    for (const [key, value] of loginResp.headers) {
      console.log(`  ${key}: ${value}`);
    }
    const body = await loginResp.text();
    console.log('Body:', body.slice(0, 1000));
  }
}

// Test C: Follow the redirect from /v2/api/login
async function testFollowRedirect() {
  console.log('\n--- Test C: /v2/api/login with redirect: follow ---');
  const params = new URLSearchParams({
    username: encUser,
    password: encPwd,
    isphone: '0',
    isEncrypted: '1',
    staticpage: 'https://pan.baidu.com/res/static/thirdparty/loginResult.html',
    charset: 'UTF-8',
    token: '',
    tpl: 'netdisk',
    apiver: 'v3',
    tt: String(Date.now()),
    u: 'https://pan.baidu.com/',
    papi: '1',
  });
  const response = await fetch('https://passport.baidu.com/v2/api/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
      Referer: 'https://pan.baidu.com/',
      Origin: 'https://pan.baidu.com',
    },
    body: params,
    redirect: 'follow',
  });
  console.log('Final status:', response.status);
  console.log('Final URL:', response.url);
  const text = await response.text();
  console.log('Body:', text.slice(0, 1500));
}

await testWebLoginRedirect();
await testGetToken();
await testFollowRedirect();
