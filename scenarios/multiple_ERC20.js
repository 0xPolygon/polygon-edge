import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import { fundTestAccounts } from './helpers/init.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';

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

const ZexCoin = JSON.parse(open("./contracts/ZexCoinERC20.json"));

export function setup() {
    let data = {};

    const client = new eth.Client({ 
        url: rpc_url,
        mnemonic: mnemonic,
    });

    const receipt = client.deployContract(JSON.stringify(ZexCoin.abi), ZexCoin.bytecode.substring(2), 500000000000, "ZexCoin", "ZEX")

    data.contract_address = receipt.contract_address;
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

export function handleSummary(data) {
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }), // Show the text summary to stdout...
        'summary.json': JSON.stringify(data),
    };
}
