import eth from 'k6/x/ethereum';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';

let rpc_url = __ENV.RPC_URL
if (rpc_url == undefined) {
  rpc_url = "http://localhost:10002"
}

const client = new eth.Client({
    url: rpc_url,
    mnemonic: __ENV.LOADTEST_MNEMONIC
});

// You can use an existing premined account
const root_address = "0x1AB8C3df809b85012a009c0264eb92dB04eD6EFa"

export function setup() {
  return { nonce: client.getNonce(root_address) };
}

export default function (data) {
  console.log(`nonce => ${data.nonce}`);
  const gas = client.gasPrice();
  console.log(`gas price => ${gas}`);

  const bal = client.getBalance(root_address, client.blockNumber());
  console.log(`bal => ${bal}`);
  
  const tx = {
    to: "0xDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF",
    value: Number(0.0001 * 1e18),
    gas_price: gas,
    nonce: data.nonce,
  };
  
  const txh = client.sendRawTransaction(tx)
  console.log("tx hash => " + txh);
  // Optional: wait for the transaction to be mined
  // const receipt = client.waitForTransactionReceipt(txh).then((receipt) => {
  //   console.log("tx block hash => " + receipt.block_hash);
  //   console.log(typeof receipt.block_number);
  // });
  data.nonce = data.nonce + 1;
}

export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }), // Show the text summary to stdout...
    'summary.json': JSON.stringify(data),
  };
}
