import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import { fundTestAccounts } from './helpers/init.js';

export const options = {
  setupTimeout: '220s',
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 200,
      timeUnit: '1s',
      duration: '1m',
      preAllocatedVUs: 200,
      maxVUs: 200,
    },
  },
};

// You can use an existing premined account
const root_address = "0x1AB8C3df809b85012a009c0264eb92dB04eD6EFa";
const mnemonic = __ENV.LOADTEST_MNEMONIC;
let rpc_url = __ENV.RPC_URL;
if (rpc_url == undefined) {
  rpc_url = "http://localhost:10002"
}

export function setup() {
  return {accounts: fundTestAccounts(root_address, rpc_url, mnemonic)};
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
    value: Number(0.0001 * 1e18),
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
