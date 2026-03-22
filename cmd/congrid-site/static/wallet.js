const config = window.CongridConfig || {};
const enabled = Boolean(config.enabled);

const slotStatus = {
  LISTED: 1,
  PAUSED: 2,
  UNLISTED: 3,
};

const dependencyURLs = {
  stargate: "https://esm.sh/@cosmjs/stargate@0.32.4",
  protoSigning: "https://esm.sh/@cosmjs/proto-signing@0.32.4",
  long: "https://esm.sh/long@5.2.3",
  protobufjs: "https://esm.sh/protobufjs@7.3.0/minimal.js",
};

let walletDepsPromise = null;

const state = {
  signer: null,
  address: "",
  client: null,
};

function makeMsgCreateSlot(_m0, Long) {
  return {
    encode(message, writer = _m0.Writer.create()) {
      if (message.publisher !== "") {
        writer.uint32(10).string(message.publisher);
      }
      if (message.domain !== "") {
        writer.uint32(18).string(message.domain);
      }
      if (message.label !== "") {
        writer.uint32(26).string(message.label);
      }
      if (message.summary !== "") {
        writer.uint32(34).string(message.summary);
      }
      if (message.category !== "") {
        writer.uint32(42).string(message.category);
      }
      if (message.placement !== "") {
        writer.uint32(50).string(message.placement);
      }
      if (message.size !== "") {
        writer.uint32(58).string(message.size);
      }
      if (message.rateDenom !== "") {
        writer.uint32(66).string(message.rateDenom);
      }
      if (message.rateAmount !== "") {
        writer.uint32(74).string(message.rateAmount);
      }
      if (!message.unitSeconds.isZero()) {
        writer.uint32(80).int64(message.unitSeconds);
      }
      if (!message.minDurationSeconds.isZero()) {
        writer.uint32(88).int64(message.minDurationSeconds);
      }
      if (!message.maxDurationSeconds.isZero()) {
        writer.uint32(96).int64(message.maxDurationSeconds);
      }
      for (const v of message.tags) {
        writer.uint32(106).string(v);
      }
      return writer;
    },
    fromPartial(object) {
      const message = {
        publisher: "",
        domain: "",
        label: "",
        summary: "",
        category: "",
        placement: "",
        size: "",
        rateDenom: "",
        rateAmount: "",
        unitSeconds: Long.ZERO,
        minDurationSeconds: Long.ZERO,
        maxDurationSeconds: Long.ZERO,
        tags: [],
      };
      message.publisher = object.publisher ?? "";
      message.domain = object.domain ?? "";
      message.label = object.label ?? "";
      message.summary = object.summary ?? "";
      message.category = object.category ?? "";
      message.placement = object.placement ?? "";
      message.size = object.size ?? "";
      message.rateDenom = object.rateDenom ?? "";
      message.rateAmount = object.rateAmount ?? "";
      message.unitSeconds = object.unitSeconds !== undefined && object.unitSeconds !== null ? Long.fromValue(object.unitSeconds) : Long.ZERO;
      message.minDurationSeconds =
        object.minDurationSeconds !== undefined && object.minDurationSeconds !== null ? Long.fromValue(object.minDurationSeconds) : Long.ZERO;
      message.maxDurationSeconds =
        object.maxDurationSeconds !== undefined && object.maxDurationSeconds !== null ? Long.fromValue(object.maxDurationSeconds) : Long.ZERO;
      message.tags = object.tags?.map((e) => e) || [];
      return message;
    },
  };
}

function makeMsgUpdateSlotStatus(_m0) {
  return {
    encode(message, writer = _m0.Writer.create()) {
      if (message.publisher !== "") {
        writer.uint32(10).string(message.publisher);
      }
      if (message.slotId !== "") {
        writer.uint32(18).string(message.slotId);
      }
      if (message.status !== 0) {
        writer.uint32(24).int32(message.status);
      }
      return writer;
    },
    fromPartial(object) {
      const message = { publisher: "", slotId: "", status: 0 };
      message.publisher = object.publisher ?? "";
      message.slotId = object.slotId ?? "";
      message.status = object.status ?? 0;
      return message;
    },
  };
}

function makeMsgLeaseSlot(_m0, Long) {
  return {
    encode(message, writer = _m0.Writer.create()) {
      if (message.lessee !== "") {
        writer.uint32(10).string(message.lessee);
      }
      if (message.slotId !== "") {
        writer.uint32(18).string(message.slotId);
      }
      if (message.targetUrl !== "") {
        writer.uint32(26).string(message.targetUrl);
      }
      if (!message.startsAtUnix.isZero()) {
        writer.uint32(32).int64(message.startsAtUnix);
      }
      if (!message.durationSeconds.isZero()) {
        writer.uint32(40).int64(message.durationSeconds);
      }
      return writer;
    },
    fromPartial(object) {
      const message = {
        lessee: "",
        slotId: "",
        targetUrl: "",
        startsAtUnix: Long.ZERO,
        durationSeconds: Long.ZERO,
      };
      message.lessee = object.lessee ?? "";
      message.slotId = object.slotId ?? "";
      message.targetUrl = object.targetUrl ?? "";
      message.startsAtUnix = object.startsAtUnix !== undefined && object.startsAtUnix !== null ? Long.fromValue(object.startsAtUnix) : Long.ZERO;
      message.durationSeconds =
        object.durationSeconds !== undefined && object.durationSeconds !== null ? Long.fromValue(object.durationSeconds) : Long.ZERO;
      return message;
    },
  };
}

function makeMsgRegisterPublisher(_m0) {
  return {
    encode(message, writer = _m0.Writer.create()) {
      if (message.owner !== "") {
        writer.uint32(10).string(message.owner);
      }
      if (message.domain !== "") {
        writer.uint32(18).string(message.domain);
      }
      if (message.metadataUri !== "") {
        writer.uint32(26).string(message.metadataUri);
      }
      if (message.verifier !== "") {
        writer.uint32(34).string(message.verifier);
      }
      if (message.referrer !== "") {
        writer.uint32(42).string(message.referrer);
      }
      return writer;
    },
    fromPartial(object) {
      const message = {
        owner: "",
        domain: "",
        metadataUri: "",
        verifier: "",
        referrer: "",
      };
      message.owner = object.owner ?? "";
      message.domain = object.domain ?? "";
      message.metadataUri = object.metadataUri ?? "";
      message.verifier = object.verifier ?? "";
      message.referrer = object.referrer ?? "";
      return message;
    },
  };
}

function dependencyLoadMessage(err) {
  const message = err?.message || String(err);
  const lowered = message.toLowerCase();
  if (
    lowered.includes("failed to fetch dynamically imported module") ||
    lowered.includes("error loading dynamically imported module") ||
    lowered.includes("importing a module script failed")
  ) {
    return "Wallet transaction support failed to load. Check your network, disable blocking extensions for this site, and retry.";
  }
  return message;
}

async function loadWalletDeps() {
  if (walletDepsPromise) {
    return walletDepsPromise;
  }

  walletDepsPromise = (async () => {
    const [stargate, protoSigning, longMod, protobufMod] = await Promise.all([
      import(dependencyURLs.stargate),
      import(dependencyURLs.protoSigning),
      import(dependencyURLs.long),
      import(dependencyURLs.protobufjs),
    ]);
    const _m0 = protobufMod.default || protobufMod;

    const Long = longMod.default;
    if (!Long) {
      throw new Error("Failed to load Long dependency.");
    }

    if (_m0.util.Long !== Long) {
      _m0.util.Long = Long;
      _m0.configure();
    }

    const registry = new protoSigning.Registry(stargate.defaultRegistryTypes);
    registry.register("/contentgrid.registry.v1.MsgCreateSlot", makeMsgCreateSlot(_m0, Long));
    registry.register("/contentgrid.registry.v1.MsgUpdateSlotStatus", makeMsgUpdateSlotStatus(_m0));
    registry.register("/contentgrid.registry.v1.MsgLeaseSlot", makeMsgLeaseSlot(_m0, Long));
    registry.register("/contentgrid.registry.v1.MsgRegisterPublisher", makeMsgRegisterPublisher(_m0));

    return {
      SigningStargateClient: stargate.SigningStargateClient,
      GasPrice: stargate.GasPrice,
      calculateFee: stargate.calculateFee,
      Long,
      registry,
    };
  })().catch((err) => {
    walletDepsPromise = null;
    throw new Error(dependencyLoadMessage(err));
  });

  return walletDepsPromise;
}

function showFlash(message, isError = false, context = document) {
  let flash = context.querySelector("[data-wallet-flash]");
  if (!flash) {
    flash = document.querySelector("[data-wallet-flash]");
  }
  if (!flash) {
    if (message) {
      alert(message);
    }
    return;
  }
  flash.textContent = message;
  flash.hidden = !message;
  flash.classList.toggle("is-error", isError);
}

function updateWalletAddress(address) {
  document.querySelectorAll("[data-wallet-address]").forEach((el) => {
    el.textContent = address || "Not connected";
  });
}

function getWalletProviders() {
  const providers = [];
  if (window.keplr) providers.push({ name: "Keplr", provider: window.keplr });
  if (window.leap) providers.push({ name: "Leap", provider: window.leap });
  return providers;
}

function toHttpRPC(rpc) {
  const raw = String(rpc || "").trim();
  if (!raw) return "";
  if (raw.startsWith("tcp://")) {
    return "http://" + raw.slice("tcp://".length);
  }
  return raw;
}

function defaultRestFromRPC(rpc) {
  try {
    const url = new URL(toHttpRPC(rpc));
    if (url.port === "26657") {
      url.port = "1317";
    }
    return url.toString().replace(/\/$/, "");
  } catch (_err) {
    return "http://127.0.0.1:1317";
  }
}

async function suggestChainIfSupported(provider) {
  if (!provider || typeof provider.experimentalSuggestChain !== "function") {
    return;
  }
  const chainId = String(config.chain_id || "").trim();
  if (!chainId) return;
  const feeDenom = String(config.fee_denom || "ucongrid").trim() || "ucongrid";
  const rpc = toHttpRPC(config.rpc || "");
  const rest = String(config.rest || defaultRestFromRPC(config.rpc || "")).trim();

  const chainInfo = {
    chainId,
    chainName: `Congrid (${chainId})`,
    rpc,
    rest,
    bip44: { coinType: 118 },
    bech32Config: {
      bech32PrefixAccAddr: "congrid",
      bech32PrefixAccPub: "congridpub",
      bech32PrefixValAddr: "congridvaloper",
      bech32PrefixValPub: "congridvaloperpub",
      bech32PrefixConsAddr: "congridvalcons",
      bech32PrefixConsPub: "congridvalconspub",
    },
    stakeCurrency: {
      coinDenom: "CONGRID",
      coinMinimalDenom: feeDenom,
      coinDecimals: 6,
    },
    currencies: [{
      coinDenom: "CONGRID",
      coinMinimalDenom: feeDenom,
      coinDecimals: 6,
    }],
    feeCurrencies: [{
      coinDenom: "CONGRID",
      coinMinimalDenom: feeDenom,
      coinDecimals: 6,
      gasPriceStep: { low: 0.001, average: 0.0025, high: 0.004 },
    }],
  };

  await provider.experimentalSuggestChain(chainInfo);
}

async function ensureWalletConnected() {
  if (!enabled) {
    throw new Error("Wallet signing is not enabled on this deployment.");
  }
  if (state.signer && state.address) {
    return state;
  }
  const providers = getWalletProviders();
  if (providers.length === 0) {
    throw new Error("Wallet extension not detected (Keplr/Leap).");
  }
  if (!config.chain_id || !config.rpc) {
    throw new Error("Missing chain configuration.");
  }

  let lastErr = null;
  for (const item of providers) {
    const provider = item.provider;
    try {
      await suggestChainIfSupported(provider);
      await provider.enable(config.chain_id);
      const signer = provider.getOfflineSigner
        ? provider.getOfflineSigner(config.chain_id)
        : await provider.getOfflineSignerAuto(config.chain_id);
      const accounts = await signer.getAccounts();
      if (!accounts || accounts.length === 0) {
        throw new Error(`No wallet accounts available in ${item.name}.`);
      }
      state.signer = signer;
      state.address = accounts[0].address;
      updateWalletAddress(state.address);
      return state;
    } catch (err) {
      lastErr = err;
    }
  }

  throw new Error(lastErr?.message || "Wallet connect failed. Please add Congrid chain to Keplr/Leap and retry.");
}

async function getClient() {
  await ensureWalletConnected();
  if (state.client) {
    return state.client;
  }

  const rpc = toHttpRPC(config.rpc || "");
  if (!rpc) {
    throw new Error("Missing RPC endpoint.");
  }

  const { SigningStargateClient, GasPrice, registry } = await loadWalletDeps();
  const gasPrice = GasPrice.fromString(config.gas_price || "0.001ucongrid");
  state.client = await SigningStargateClient.connectWithSigner(rpc, state.signer, {
    registry,
    gasPrice,
  });
  return state.client;
}

function parseStartDate(value) {
  if (!value) {
    return 0;
  }
  const parts = value.split("-");
  if (parts.length !== 3) {
    throw new Error("Invalid start date.");
  }
  const year = Number(parts[0]);
  const month = Number(parts[1]) - 1;
  const day = Number(parts[2]);
  const startMs = Date.UTC(year, month, day, 0, 0, 0, 0);
  if (Number.isNaN(startMs)) {
    throw new Error("Invalid start date.");
  }
  const nowMs = Date.now();
  if (startMs < nowMs - 24 * 60 * 60 * 1000) {
    throw new Error("Start date must be today or later.");
  }
  let effectiveMs = startMs;
  if (startMs < nowMs) {
    effectiveMs = nowMs + 5 * 60 * 1000;
  }
  return Math.floor(effectiveMs / 1000);
}

async function submitTx(msgs, gasLimit) {
  const client = await getClient();
  const { GasPrice, calculateFee } = await loadWalletDeps();
  const gasPrice = GasPrice.fromString(config.gas_price || "0.001ucongrid");
  const fee = calculateFee(gasLimit, gasPrice);
  const result = await client.signAndBroadcast(state.address, msgs, fee, "");
  if (result.code && result.code !== 0) {
    throw new Error(result.rawLog || `Tx failed with code ${result.code}`);
  }
  return result.transactionHash;
}

function bindConnectButtons() {
  document.querySelectorAll("[data-wallet-connect]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      try {
        await ensureWalletConnected();
        showFlash(`Connected: ${state.address}`);
      } catch (err) {
        showFlash(err.message || String(err), true);
      }
    });
  });
}

function bindCreateSlotForms() {
  document.querySelectorAll("form[data-wallet-slot-create]").forEach((form) => {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        await ensureWalletConnected();
        const { Long } = await loadWalletDeps();
        const data = new FormData(form);
        const domain = String(data.get("publisher") || "").trim();
        const label = String(data.get("label") || "").trim();
        const summary = String(data.get("summary") || "").trim();
        const category = String(data.get("category") || "").trim();
        const placement = String(data.get("placement") || "").trim();
        const size = String(data.get("size") || "").trim();
        const rateRaw = String(data.get("rate") || "").trim();
        if (!domain) {
          throw new Error("Publisher domain required.");
        }
        if (!label) {
          throw new Error("Slot label required.");
        }
        if (!rateRaw) {
          throw new Error("Rate required.");
        }
        const rateAmount = Number(rateRaw);
        if (!Number.isFinite(rateAmount) || rateAmount < 0 || !Number.isInteger(rateAmount)) {
          throw new Error("Rate must be a non-negative integer.");
        }
        const msg = {
          typeUrl: "/contentgrid.registry.v1.MsgCreateSlot",
          value: {
            publisher: state.address,
            domain,
            label,
            summary,
            category,
            placement,
            size,
            rateDenom: config.slot_rate_denom || "ucongrid",
            rateAmount: String(rateAmount),
            unitSeconds: Long.fromValue(config.slot_unit_seconds || 0),
            minDurationSeconds: Long.fromValue(config.slot_min_duration_seconds || 0),
            maxDurationSeconds: Long.fromValue(config.slot_max_duration_seconds || 0),
            tags: [],
          },
        };
        const txHash = await submitTx([msg], config.gas_create_slot || 220000);
        showFlash(`Slot created. Tx: ${txHash}`);
        setTimeout(() => window.location.reload(), 1500);
      } catch (err) {
        showFlash(err.message || String(err), true);
      }
    });
  });
}

function bindSlotStatusForms() {
  document.querySelectorAll("form[data-wallet-slot-status]").forEach((form) => {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        await ensureWalletConnected();
        const data = new FormData(form);
        const slotId = String(data.get("slot_id") || "").trim();
        const action = event.submitter ? event.submitter.value : String(data.get("action") || "").trim();
        if (!slotId) {
          throw new Error("Slot id required.");
        }
        let status = 0;
        switch (action) {
          case "activate":
            status = slotStatus.LISTED;
            break;
          case "pause":
            status = slotStatus.PAUSED;
            break;
          case "unlist":
            status = slotStatus.UNLISTED;
            break;
          default:
            throw new Error("Unknown slot action.");
        }
        const msg = {
          typeUrl: "/contentgrid.registry.v1.MsgUpdateSlotStatus",
          value: {
            publisher: state.address,
            slotId,
            status,
          },
        };
        const txHash = await submitTx([msg], config.gas_update_slot || 140000);
        showFlash(`Slot updated. Tx: ${txHash}`);
        setTimeout(() => window.location.reload(), 1500);
      } catch (err) {
        showFlash(err.message || String(err), true);
      }
    });
  });
}

function bindLeaseForms() {
  document.querySelectorAll("form[data-wallet-lease]").forEach((form) => {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        await ensureWalletConnected();
        const { Long } = await loadWalletDeps();
        const data = new FormData(form);
        const slotId = String(data.get("slot_id") || "").trim();
        const targetUrl = String(data.get("target_url") || "").trim();
        const durationRaw = String(data.get("duration_seconds") || "").trim();
        const startDate = String(data.get("start_date") || "").trim();
        if (!slotId || !targetUrl) {
          throw new Error("Slot and target URL are required.");
        }
        const durationSeconds = Number(durationRaw);
        if (!Number.isFinite(durationSeconds) || durationSeconds <= 0 || !Number.isInteger(durationSeconds)) {
          throw new Error("Select a lease duration.");
        }
        const startsAtUnix = parseStartDate(startDate);
        const msg = {
          typeUrl: "/contentgrid.registry.v1.MsgLeaseSlot",
          value: {
            lessee: state.address,
            slotId,
            targetUrl,
            startsAtUnix: Long.fromValue(startsAtUnix),
            durationSeconds: Long.fromValue(durationSeconds),
          },
        };
        const txHash = await submitTx([msg], config.gas_lease_slot || 220000);
        showFlash(`Lease requested. Tx: ${txHash}`);
        setTimeout(() => window.location.reload(), 1500);
      } catch (err) {
        showFlash(err.message || String(err), true);
      }
    });
  });
}

function bindPublisherRegisterForms() {
  document.querySelectorAll("form[data-wallet-publisher-register]").forEach((form) => {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        await ensureWalletConnected();
        const data = new FormData(form);
        const domain = String(data.get("domain") || "").trim().toLowerCase();
        const wallet = String(data.get("wallet") || "").trim();
        if (!domain) {
          throw new Error("Please generate registration details first (missing domain).");
        }
        if (!wallet) {
          throw new Error("Please generate registration details first (missing wallet).");
        }
        if (wallet !== state.address) {
          throw new Error(`Connected wallet mismatch. connected=${state.address} form=${wallet}`);
        }

        const msg = {
          typeUrl: "/contentgrid.registry.v1.MsgRegisterPublisher",
          value: {
            owner: state.address,
            domain,
            metadataUri: "",
            verifier: "",
            referrer: "",
          },
        };
        const txHash = await submitTx([msg], 220000);
        showFlash(`Publisher registered. Tx: ${txHash}`, false, form);
      } catch (err) {
        console.error("Wallet Action Error:", err);
        showFlash(err.message || String(err), true, form);
      }
    });
  });
}

function initWalletUI() {
  if (!enabled) {
    return;
  }
  updateWalletAddress("");
  bindConnectButtons();
  bindCreateSlotForms();
  bindSlotStatusForms();
  bindLeaseForms();
  bindPublisherRegisterForms();
  window.CongridWalletUIReady = true;
}

initWalletUI();
