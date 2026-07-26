#!/usr/bin/env node
// Mirrors VERSION=1 payment URL builder from the managed template overlay.
// Used only by isolated tests (not shipped to SHM).

export function buildPaymentHref({
  shm_url,
  amount,
  email = "",
  brandID = null,
  yookassaPaySystem = null,
  forExternal = false,
  origin = "https://bill.example",
}) {
  const rawPaymentURL = shm_url + amount;
  let paymentURL;
  try {
    paymentURL = new URL(rawPaymentURL, origin);
  } catch (e) {
    const err = new Error("malformed_url");
    err.code = "malformed_url";
    throw err;
  }
  const actualPs = paymentURL.searchParams.get("ps");
  if (brandID) {
    if (!yookassaPaySystem) {
      const err = new Error("partial_config");
      err.code = "partial_config";
      throw err;
    }
    if (actualPs === yookassaPaySystem) {
      paymentURL.searchParams.set("brand_id", brandID);
    }
  }
  if (forExternal) {
    paymentURL.searchParams.set("email", email || "");
  }
  return paymentURL.toString();
}

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

function run() {
  const yk =
    "https://bill.example/shm/pay_systems/yookassa.cgi?action=create&user_id=1&ps=yookassa&amount=";
  const cc =
    "https://bill.example/shm/pay_systems/cryptocloud.cgi?action=create&user_id=1&ps=cryptocloud&amount=";

  let u = new URL(
    buildPaymentHref({
      shm_url: yk,
      amount: "199",
      brandID: "vff",
      yookassaPaySystem: "yookassa",
      forExternal: true,
      email: "a@b.c",
    })
  );
  assert(u.searchParams.get("brand_id") === "vff", "vff brand_id");
  assert(u.searchParams.get("ps") === "yookassa", "vff ps");
  assert(u.searchParams.get("amount") === "199", "amount not glued to brand");
  assert(u.searchParams.get("email") === "a@b.c", "email");

  u = new URL(
    buildPaymentHref({
      shm_url: yk,
      amount: "50",
      brandID: "fc",
      yookassaPaySystem: "yookassa",
      forExternal: true,
      email: "x@y.z",
    })
  );
  assert(u.searchParams.get("brand_id") === "fc", "fc brand_id");

  u = new URL(
    buildPaymentHref({
      shm_url: cc,
      amount: "10",
      brandID: "vff",
      yookassaPaySystem: "yookassa",
      forExternal: true,
      email: "a@b.c",
    })
  );
  assert(u.searchParams.get("brand_id") === null, "cryptocloud no brand_id");
  assert(u.searchParams.get("ps") === "cryptocloud", "cryptocloud ps");

  const legacy = buildPaymentHref({
    shm_url: yk,
    amount: "12",
    brandID: null,
    yookassaPaySystem: null,
    forExternal: true,
    email: "a+b@ex.com",
  });
  u = new URL(legacy);
  assert(u.searchParams.get("brand_id") === null, "legacy no brand_id");
  assert(u.searchParams.get("email") === "a+b@ex.com", "email encoded");
  assert(legacy.includes("a%2Bb%40ex.com") || u.searchParams.get("email") === "a+b@ex.com", "email special");

  // relative shm_url
  u = new URL(
    buildPaymentHref({
      shm_url: "/shm/pay_systems/yookassa.cgi?action=create&ps=yookassa&amount=",
      amount: "7",
      brandID: "vff",
      yookassaPaySystem: "yookassa",
      origin: "https://bill.example",
    })
  );
  assert(u.origin === "https://bill.example", "relative origin");
  assert(u.searchParams.get("brand_id") === "vff", "relative brand");

  let threw = false;
  try {
    buildPaymentHref({ shm_url: "http://[::invalid", amount: "1" });
  } catch (e) {
    threw = e.code === "malformed_url";
  }
  assert(threw, "malformed url");

  threw = false;
  try {
    buildPaymentHref({
      shm_url: yk,
      amount: "1",
      brandID: "vff",
      yookassaPaySystem: null,
    });
  } catch (e) {
    threw = e.code === "partial_config";
  }
  assert(threw, "partial config fail-closed");

  console.log("OK payment_url_logic");
}

run();
