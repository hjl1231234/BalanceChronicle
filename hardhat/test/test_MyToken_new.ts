import { expect } from "chai";
import { ethers } from "hardhat";
import { Signer } from "ethers";
import type { MyToken } from "../typechain-types";

describe("MyToken", function () {
  let myToken: MyToken;
  let owner: Signer;
  let addr1: Signer;
  let addr2: Signer;

  beforeEach(async function () {
    [owner, addr1, addr2] = await ethers.getSigners();

    const MyToken = await ethers.getContractFactory("MyToken");
    myToken = await MyToken.deploy(await owner.getAddress()) as MyToken;
    await myToken.waitForDeployment();
  });

  it("Should set the right owner", async function () {
    expect(await myToken.owner()).to.equal(await owner.getAddress());
  });

  it("Should have correct initial supply", async function () {
    expect(await myToken.totalSupply()).to.equal(0);
  });

  it("Should have correct name and symbol", async function () {
    expect(await myToken.name()).to.equal("RDemoToken");
    expect(await myToken.symbol()).to.equal("RDT");
  });

  it("Should allow minting tokens by sending ETH", async function () {
    const ethAmount = ethers.parseEther("0.01");
    const expectedTokens = ethAmount * BigInt(100000000); // 100,000,000 tokens per ETH

    await expect(
      myToken.connect(addr1).mint({ value: ethAmount })
    ).to.changeTokenBalance(myToken, addr1, expectedTokens);

    expect(await ethers.provider.getBalance(await myToken.getAddress())).to.equal(ethAmount);
  });

  it("Should revert minting with insufficient ETH", async function () {
    const ethAmount = ethers.parseEther("0.0005"); // Less than MIN_ETH (0.001 ETH)

    await expect(
      myToken.connect(addr1).mint({ value: ethAmount })
    ).to.be.revertedWith("Not enough ETH sent");
  });

  it("Should allow direct ETH transfers to mint tokens", async function () {
    const ethAmount = ethers.parseEther("0.02");
    const expectedTokens = ethAmount * BigInt(100000000);

    await expect(
      addr1.sendTransaction({ to: await myToken.getAddress(), value: ethAmount })
    ).to.changeTokenBalance(myToken, addr1, expectedTokens);
  });

  it("Should allow owner to withdraw ETH", async function () {
    const ethAmount = ethers.parseEther("0.01");
    
    // Mint tokens first to add ETH to the contract
    await myToken.connect(addr1).mint({ value: ethAmount });
    
    const initialOwnerBalance = await ethers.provider.getBalance(await owner.getAddress());
    
    const withdrawTx = await myToken.connect(owner).withdrawETH();
    const withdrawReceipt = await withdrawTx.wait();
    const gasUsed = withdrawReceipt!.gasUsed * withdrawReceipt!.gasPrice;
    
    const finalOwnerBalance = await ethers.provider.getBalance(await owner.getAddress());
    
    expect(finalOwnerBalance).to.be.closeTo(
      initialOwnerBalance + ethAmount, 
      gasUsed
    );
    expect(await ethers.provider.getBalance(await myToken.getAddress())).to.equal(0);
  });

  it("Should not allow non-owner to withdraw ETH", async function () {
    const ethAmount = ethers.parseEther("0.01");
    
    // Mint tokens first to add ETH to the contract
    await myToken.connect(addr1).mint({ value: ethAmount });
    
    await expect(
      myToken.connect(addr1).withdrawETH()
    ).to.be.revertedWithCustomError(myToken, "OwnableUnauthorizedAccount");
  });

  it("Should revert withdraw when contract has no ETH", async function () {
    await expect(
      myToken.connect(owner).withdrawETH()
    ).to.be.revertedWith("No ETH to withdraw");
  });

  describe("New Features: Mint, Burn, Batch Operations", function () {
    it("Should allow owner to mintTo a specific address", async function () {
      const mintAmount = ethers.parseUnits("1000", 18);
      await myToken.connect(owner).mintTo(await addr1.getAddress(), mintAmount);
      expect(await myToken.balanceOf(await addr1.getAddress())).to.equal(mintAmount);
    });

    it("Should revert if non-owner tries to mintTo", async function () {
      const mintAmount = ethers.parseUnits("1000", 18);
      await expect(
        myToken.connect(addr1).mintTo(await addr2.getAddress(), mintAmount)
      ).to.be.revertedWithCustomError(myToken, "OwnableUnauthorizedAccount");
    });

    it("Should allow user to burn their own tokens", async function () {
      const mintAmount = ethers.parseUnits("1000", 18);
      const burnAmount = ethers.parseUnits("400", 18);

      await myToken.connect(owner).mintTo(await addr1.getAddress(), mintAmount);
      await myToken.connect(addr1).burn(burnAmount);

      expect(await myToken.balanceOf(await addr1.getAddress())).to.equal(mintAmount - burnAmount);
    });

    it("Should allow owner to batchMint", async function () {
      const amounts = [ethers.parseUnits("100", 18), ethers.parseUnits("200", 18)];
      const recipients = [await addr1.getAddress(), await addr2.getAddress()];

      await myToken.connect(owner).batchMint(recipients, amounts);

      expect(await myToken.balanceOf(await addr1.getAddress())).to.equal(amounts[0]);
      expect(await myToken.balanceOf(await addr2.getAddress())).to.equal(amounts[1]);
    });

    it("Should allow user to batchTransfer", async function () {
      // 1. Mint 1000 to addr1
      const mintAmount = ethers.parseUnits("1000", 18);
      await myToken.connect(owner).mintTo(await addr1.getAddress(), mintAmount);

      // 2. Addr1 batch transfers to owner and addr2
      const amounts = [ethers.parseUnits("100", 18), ethers.parseUnits("200", 18)];
      const recipients = [await owner.getAddress(), await addr2.getAddress()];

      await myToken.connect(addr1).batchTransfer(recipients, amounts);

      expect(await myToken.balanceOf(await addr1.getAddress())).to.equal(mintAmount - amounts[0] - amounts[1]);
      expect(await myToken.balanceOf(await owner.getAddress())).to.equal(amounts[0]);
      expect(await myToken.balanceOf(await addr2.getAddress())).to.equal(amounts[1]);
    });

    it("Should allow approved user to burnFrom", async function () {
      const mintAmount = ethers.parseUnits("1000", 18);
      const burnAmount = ethers.parseUnits("300", 18);

      // owner mints to addr1
      await myToken.connect(owner).mintTo(await addr1.getAddress(), mintAmount);

      // addr1 approves addr2
      await myToken.connect(addr1).approve(await addr2.getAddress(), burnAmount);

      // addr2 burns from addr1
      await myToken.connect(addr2).burnFrom(await addr1.getAddress(), burnAmount);

      expect(await myToken.balanceOf(await addr1.getAddress())).to.equal(mintAmount - burnAmount);
      // check allowance is consumed
      expect(await myToken.allowance(await addr1.getAddress(), await addr2.getAddress())).to.equal(0);
    });
  });
});
