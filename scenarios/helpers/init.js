import eth from 'k6/x/ethereum';
import exec from 'k6/execution';
import wallet from 'k6/x/ethereum/wallet';

export function fundTestAccounts(root_address, url, priv_key) {
    const client = new eth.Client({ 
        url: url,
        privateKey: priv_key,
    });
    var accounts = [];
    var nonce = client.getNonce(root_address);

    // fund the VUs accounts
    for (let i = 0; i < exec.instance.vusInitialized; i++) {
        var tacc = wallet.generateKey();
        accounts[i] = {
            private_key: tacc.private_key,
            address: tacc.address,
        };

        // fund each account with 5 ETH
        var tx = {
            to: tacc.address,
            value: Number(0.05 * 1e18),
            gas_price: client.gasPrice(),
            nonce: nonce,
        };

        console.log(JSON.stringify(tx));
        var txh = client.sendRawTransaction(tx)
        client.waitForTransactionReceipt(txh).then((receipt) => {
            console.log(`account funded => ${receipt.block_hash}`);
        });

        nonce++;
    }

    return accounts;
}
