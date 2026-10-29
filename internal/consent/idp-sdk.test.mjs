import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

function loadSDK() {
  const context = vm.createContext({});
  for (const name of ['console', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'URL', 'URLSearchParams',
    'TextEncoder', 'TextDecoder', 'crypto', 'fetch', 'atob', 'btoa', 'queueMicrotask', 'structuredClone',
    'AbortController', 'Blob', 'Event', 'EventTarget']) {
    context[name] = globalThis[name];
  }
  context.HTMLElement = class {};
  context.window = context;
  context.self = context;
  vm.runInContext(readFileSync(new URL('./idp-sdk.js', import.meta.url), 'utf8'), context);
  return context.stytch;
}

test('the SDK defines the global the page reads', () => {
  assert.equal(typeof loadSDK(), 'object');
});

test('the SDK exports the client and the custom elements the page defines', () => {
  const sdk = loadSDK();
  for (const name of ['StytchClient', 'StytchUI', 'StytchIdentityProvider']) {
    assert.equal(typeof sdk[name], 'function', name);
  }
});

test('the SDK exports the OAuth product and the Discord provider', () => {
  const sdk = loadSDK();
  assert.equal(typeof sdk.Products?.oauth, 'object');
  assert.equal(sdk.OAuthProviders?.Discord, 'discord');
});
