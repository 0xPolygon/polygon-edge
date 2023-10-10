import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import { fundTestAccounts } from '../helpers/init.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';

let duration = __ENV.LOADTEST_DURATION;
if (duration == undefined) {
    duration = "2m";
}

export const options = {
    setupTimeout: '220s',
    scenarios: {
        constant_request_rate: {
            executor: 'constant-arrival-rate',
            rate: 1500,
            timeUnit: '1s',
            duration: duration,
            preAllocatedVUs: 60,
            maxVUs: 60,
        },
    },
};

// You can use an existing premined account
const root_address = "0x1AB8C3df809b85012a009c0264eb92dB04eD6EFa";
const mnemonic = __ENV.LOADTEST_MNEMONIC;
let rpc_url = __ENV.RPC_URL;
if (rpc_url == undefined) {
    rpc_url = "http://localhost:10002";
}

const ZexCoin = JSON.parse(open("../contracts/ZexCoinERC20.json"));

export function setup() {
    let data = {};

    const client = new eth.Client({
        url: rpc_url,
        mnemonic: mnemonic,
    });

    const receipt = client.deployContract(JSON.stringify(ZexCoin.abi), ZexCoin.bytecode.substring(2), 500000000000, "ZexCoin", "ZEX")

    return {
        accounts: fundTestAccounts(client, root_address),
        contract_address: data.contract_address
    };
}

let nonce = 0;
let client;

// VU client
export default function (data) {
    let acc = data.accounts[exec.vu.idInInstance - 1];

    if (client == null) {
        client = new eth.Client({
            url: rpc_url,
            privateKey: acc.private_key
        });
    }

    console.log(acc.address);
    const con = client.newContract(data.contract_address, JSON.stringify(ZexCoin.abi));
    const res = con.txn("transfer", { gas_limit: 100000, nonce: nonce }, acc.address, 1);
    console.log(`txn hash => ${res}`);

    nonce++;
    // console.log(JSON.stringify(con.call("balanceOf", acc.address)));
}

export function handleSummary(data) {
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }), // Show the text summary to stdout...
        'summary.json': JSON.stringify(data),
    };
}
