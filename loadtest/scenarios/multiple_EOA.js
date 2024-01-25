import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import { fundTestAccounts } from '../helpers/init.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';

let setupTimeOut = __ENV.SETUP_TIMEOUT;
if (setupTimeOut == undefined) {
  setupTimeOut = "220s"
}

let rate = __ENV.RATE;
if (rate == undefined) {
  rate = "3000"
}

let timeUnit = __ENV.TIME_UNIT;
if (timeUnit == undefined) {
  timeUnit = "1s"
}

let duration = __ENV.DURATION;
if (duration == undefined) {
    duration = "2m";
}

let preAllocatedVUs = __ENV.PREALLOCATED_VUS;
if (preAllocatedVUs == undefined) {
  preAllocatedVUs = "60";
}

let maxVUs = __ENV.MAX_VUS;
if (maxVUs == undefined) {
  maxVUs = "60";
}

export const options = {
  setupTimeout: setupTimeOut,
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: parseInt(rate),
      timeUnit: timeUnit,
      duration: duration,
      preAllocatedVUs: parseInt(preAllocatedVUs),
      maxVUs: parseInt(maxVUs),
    },
  },
};

// You can use an existing premined account
const root_address = "0x85da99c8a7c2c95964c8efd687e95e632fc533d6";
const mnemonic = __ENV.LOADTEST_MNEMONIC;
let rpc_url = __ENV.RPC_URL;
if (rpc_url == undefined) {
  rpc_url = "http://localhost:10002"
}

export function setup() {
  const client = new eth.Client({
    url: rpc_url,
    mnemonic: mnemonic,
  });

  return { accounts: fundTestAccounts(client, root_address) };
}

var nonce = 0;
var client;

// VU client
export default function (data) {
  if (client == null) {
    client = new eth.Client({
      url: rpc_url,
      privateKey: data.accounts[exec.vu.idInInstance - 1].private_key
    });
  }

  console.log(`nonce => ${nonce}`);

  const tx = {
    to: "0xDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF",
    value: Number(0.00000001 * 1e18),
    gas_price: client.gasPrice(),
    nonce: nonce,
  };

  const txh = client.sendRawTransaction(tx);
  console.log("tx hash => " + txh);
  nonce++;

  // client.waitForTransactionReceipt(txh).then((receipt) => {
  //   console.log("tx block hash => " + receipt.block_hash);
  // });
}

export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }), // Show the text summary to stdout...
    'summary.json': JSON.stringify(data),
  };
}
