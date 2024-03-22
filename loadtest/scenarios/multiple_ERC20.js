import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import { fundTestAccounts } from '../helpers/init.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';

let setupTimeout = __ENV.SETUP_TIMEOUT;
if (setupTimeout == undefined) {
  setupTimeout = "1800s"
}

let rate = __ENV.RATE;
if (rate == undefined) {
  rate = "1500"
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
  setupTimeout: setupTimeout,
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
    rpc_url = "http://localhost:10002";
}

const ZexCoin = JSON.parse(open("../contracts/ZexCoinERC20.json"));

export async function setup() {
    let data = {};

    const client = new eth.Client({
        url: rpc_url,
        mnemonic: mnemonic,
    });

    const receipt = client.deployContract(JSON.stringify(ZexCoin.abi), ZexCoin.bytecode.substring(2), 500000000000, "ZexCoin", "ZEX")

    var accounts = await fundTestAccounts(client, root_address);

    return {
        accounts: accounts,
        contract_address: data.contract_address
    };
}

var clients = [];

// VU client
export default function (data) {
    var client = clients[exec.vu.idInInstance - 1];
    if (client == null) {
      client = new eth.Client({
        url: rpc_url,
        privateKey: data.accounts[exec.vu.idInInstance - 1].private_key
      });
  
      clients[exec.vu.idInInstance - 1] = client;
    }

    let acc = data.accounts[exec.vu.idInInstance - 1];

    console.log(acc.address);
    const con = client.newContract(data.contract_address, JSON.stringify(ZexCoin.abi));
    const res = con.txn("transfer", { gas_limit: 100000, nonce: acc.nonce, gas_price: client.gasPrice()*1.3 }, acc.address, 1);
    console.log("sender => " + acc.address + " tx hash => " + res + " nonce => " + acc.nonce);

    acc.nonce++;
    // console.log(JSON.stringify(con.call("balanceOf", acc.address)));
}

export function handleSummary(data) {
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }), // Show the text summary to stdout...
        'summary.json': JSON.stringify(data),
    };
}
