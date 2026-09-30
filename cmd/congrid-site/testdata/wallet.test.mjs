import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const catalog = JSON.parse(readFileSync(new URL("../static/translations.json", import.meta.url), "utf8"));
const i18n = readFileSync(new URL("../static/i18n.js", import.meta.url), "utf8");
const wallet = readFileSync(new URL("../static/wallet.js", import.meta.url), "utf8");

function setup(lang, providers = {}) {
  const address = { textContent: "" };
  const flash = { textContent: "", hidden: true, classList: { toggle() {} } };
  const button = { addEventListener(_, handler) { this.click = handler; } };
  const window = {
    CongridConfig: { enabled: true, chain_id: "congrid-main", rpc: "https://congrid.net/rpc" },
    CongridMessages: Object.fromEntries(Object.entries(catalog).filter(([, entry]) => entry.client)
      .map(([key, entry]) => [key, entry[lang] || key])),
    ...providers,
  };
  const document = {
    querySelector() { return flash; },
    querySelectorAll(selector) {
      if (selector === "[data-wallet-connect]") return [button];
      if (selector === "[data-wallet-address]") return [address];
      return [];
    },
  };
  const context = vm.createContext({ window, document });
  vm.runInContext(i18n, context);
  vm.runInContext(wallet, context);
  return { window, address, flash, button };
}

for (const [lang, idle, missing] of [
  ["en", "Not connected", "Wallet extension not detected (Keplr)."],
  ["zh", "未连接", "未检测到 Keplr 钱包扩展。"],
  ["fr", "Non connecté", "Extension de portefeuille Keplr non détectée."],
]) {
  test(`wallet connection notices follow ${lang}`, async () => {
    const ui = setup(lang);
    assert.equal(ui.address.textContent, idle);
    await ui.button.click();
    assert.equal(ui.flash.textContent, missing);
    assert.equal(ui.flash.hidden, false);
    assert.equal(ui.window.CongridWalletUIReady, true);
  });
}

test("Keplr connects and preserves the public address in French notices", async () => {
  const address = "congrid1fglanlkvqtyznlw3flu88680zmctyug8qr03pj";
  const enabledChains = [];
  const ui = setup("fr", {
    keplr: {
      async enable(chainId) { enabledChains.push(chainId); },
      getOfflineSigner() { return { async getAccounts() { return [{ address }]; } }; },
    },
  });
  await ui.button.click();
  assert.deepEqual(enabledChains, ["congrid-main"]);
  assert.equal(ui.address.textContent, address);
  assert.equal(ui.flash.textContent, `Connecté : ${address}`);
});

test("an obsolete provider is not accepted as a supported wallet", async () => {
  const ui = setup("zh", { leap: { enable() { throw new Error("must not be called"); } } });
  await ui.button.click();
  assert.equal(ui.flash.textContent, "未检测到 Keplr 钱包扩展。");
});

test("client interpolation preserves user values and unknown technical errors", () => {
  const { window } = setup("zh");
  assert.equal(window.CongridI18n.t("Connected: {0}", "{1}", "value"), "已连接：{1}");
  assert.equal(window.CongridI18n.t("RPC error 503"), "RPC error 503");
});
