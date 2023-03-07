import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import wallet from 'k6/x/ethereum/wallet';

export function fundTestAccounts(client, root_address) {
    var accounts = [];
    var nonce = client.getNonce(root_address);
    console.log(`nonce => ${nonce}`);

    // fund the VUs accounts
    for (let i = 0; i < exec.instance.vusInitialized; i++) {
        var tacc = wallet.generateKey();
        accounts[i] = {
            private_key: tacc.private_key,
            address: tacc.address,
        };

        // fund each account with some coins
        var tx = {
            to: tacc.address,
            value: Number(0.05 * 1e18),
            gas_price: client.gasPrice(),
            nonce: nonce,
        };

        console.log(JSON.stringify(tx));
        var txh = client.sendRawTransaction(tx)
        console.log(`txn hash => ${txh}`);
        client.waitForTransactionReceipt(txh).then((receipt) => {
            console.log(`account funded => ${JSON.stringify(receipt)}`);
        });

        nonce++;
    }

    return accounts;
}
