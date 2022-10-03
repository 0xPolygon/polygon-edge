const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("Bridge", function () {
  it("Emit a StateSync event once emitEvent function is triggered", async function () {
    const [owner] = await ethers.getSigners()

    const Bridge = await ethers.getContractFactory("RootchainBridge");
    const bridge = await Bridge.deploy();

    // emit an event and wait
    const eventTx = await bridge.emitEvent(owner.address, []);
    const receipt = await eventTx.wait();
    const stateSyncEvents = receipt.events?.filter(e => e.event == "StateSync")
    expect(stateSyncEvents).to.length(1)
    expect(stateSyncEvents[0].args[0]).to.equal(0)
    expect(stateSyncEvents[0].args[2]).to.equal(owner.address)
  });
});

describe("ERC20MintToken", function () {
  it("Should mint the token", async function () {
    const [owner] = await ethers.getSigners()

    const Token = await ethers.getContractFactory("MintERC20")
    const token = await Token.deploy()

    console.log(await token.balanceOf(owner.address))

    await token.mint(100)

    console.log(await token.balanceOf(owner.address))
  })
})

describe("ERC20BridgeWrapper", function () {
  it("Should update the token balance upon state sync", async function () {
    const [owner] = await ethers.getSigners()

    const Token = await ethers.getContractFactory("MintERC20")
    const Wrapper = await ethers.getContractFactory("ERC20Bridge")

    const token = await Token.deploy()
    const wrapper = await Wrapper.deploy(token.address)

    const data = ethers.utils.defaultAbiCoder.encode(["address", "uint256"], [owner.address, 100])

    // now call state sync in wrapper and the balance of token should change
    await wrapper.onStateReceive(0, owner.address, data)

    console.log(await token.balanceOf(owner.address))
  })
})

describe("SidechainBridge", function () {
  let erc20Token;
  let erc20BridgeWrapper;
  let sidechainBridge;

  beforeEach(async function () {
    const Erc20Token = await ethers.getContractFactory("MintERC20")
    erc20Token = await Erc20Token.deploy()

    const Erc20BridgeWrapper = await ethers.getContractFactory("ERC20Bridge")
    erc20BridgeWrapper = await Erc20BridgeWrapper.deploy(erc20Token.address)

    const SidechainBridge = await ethers.getContractFactory("SidechainBridge")
    sidechainBridge = await SidechainBridge.deploy()
  })

  it("[Register commitment] Increment next execution state sync index and transfer tokens to the receiver", async function () {
    const signature = { "aggregatedSignature": [], "bitmap": [] }
    const startIndex = 0
    const endIndex = 20
    const bundleSize = 5
    const commitment = {
      "merkleRoot": ethers.constants.HashZero,
      "fromIndex": startIndex,
      "toIndex": endIndex,
      "bundleSize": bundleSize,
      "epoch": 0,
    }
    await sidechainBridge.registerCommitment(commitment, signature)
    const nextCommitedIndex = await sidechainBridge.getNextCommittedIndex()
    expect(nextCommitedIndex).to.equal(endIndex + 1);
  })

  it("[Execute bundle] Increment next execution state sync index and transfer tokens to the receiver", async function () {
    const [sender, receiver] = await ethers.getSigners()
    const amount = 100;
    const stateSyncsCount = 10;

    const ResultStatus = {
      Success: 0,
      Failure: 1,
    }

    const tokenContractData = ethers.utils.defaultAbiCoder.encode(
      ["address", "uint256"],
      [receiver.address, amount]);

    let stateSyncs = new Array(stateSyncsCount)
    for (let i = 0; i < stateSyncsCount; i++) {
      stateSyncs[i] = {
        "id": i,
        "sender": sender.address,
        "target": erc20BridgeWrapper.address,
        "data": tokenContractData
      }
    }
    console.log("Sender: %s", sender.address)
    console.log("Receiver: %s", receiver.address)

    const tx = await sidechainBridge.executeBundle([], stateSyncs)
    const receipt = await tx.wait()
    const resultEvents = receipt.events?.filter(e => e.event == "ResultEvent")
    console.log("Result events: %s", resultEvents.length)
    expect(resultEvents).to.length(stateSyncsCount)
    let i = 0;
    for (const event of resultEvents) {
      expect(event.args[0]).to.equal(i);
      expect(event.args[2]).to.equal(ResultStatus.Success)
      i++;
    }

    console.log("'%s' balance=%s", receiver.address, await erc20Token.balanceOf(receiver.address))
    expect(await erc20Token.balanceOf(receiver.address)).to.equal(stateSyncsCount * amount)

    const nextExecutionIdx = await sidechainBridge.getNextExecutionIndex()
    console.log("Pending state sync index: %s", nextExecutionIdx)
    expect(nextExecutionIdx).to.equal(stateSyncsCount)
  })
})
