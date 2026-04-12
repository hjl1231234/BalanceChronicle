import { ethers, network, run } from "hardhat";

async function main() {
  const [deployer] = await ethers.getSigners();

  console.log(
    "Deploying MyToken contract with the account:",
    deployer.address
  );
  
  console.log("Account balance:", (await ethers.provider.getBalance(deployer.address)).toString());

  const MyToken = await ethers.getContractFactory("MyToken");
  const myToken = await MyToken.deploy(deployer.address);
  
  // Wait for the contract to be deployed
  await myToken.waitForDeployment();
  // Wait for 5 confirmations (recommended for Etherscan verification)
  const deploymentTx = await myToken.deploymentTransaction();
  if (deploymentTx) {
    await deploymentTx.wait(5);
  }

  const contractAddress = await myToken.getAddress();
  console.log("MyToken contract address:", contractAddress);

  // 自动保存合约地址到 deployments.json，供 Tasks 读取
  const fs = require("fs");
  const path = require("path");
  const deploymentsPath = path.join(__dirname, "../deployments.json");
  let deployments: any = {};
  if (fs.existsSync(deploymentsPath)) {
    deployments = JSON.parse(fs.readFileSync(deploymentsPath, "utf8"));
  }
  deployments[network.name] = contractAddress;
  fs.writeFileSync(deploymentsPath, JSON.stringify(deployments, null, 2));
  console.log(`✅ 合约地址已自动保存到 deployments.json (${network.name})`);

  // Verify contract on Etherscan if deployed to Sepolia
  if (network.name === "sepolia") {
    try {
      console.log("Verifying contract on Etherscan...");
      await run("verify:verify", {
        address: await myToken.getAddress(),
        constructorArguments: [deployer.address],
      });
      console.log("Verification successful!");
    } catch (error: any) {
      if (error.message.toLowerCase().includes("already verified")) {
        console.log("Contract already verified!");
      } else {
        console.error("Verification failed:", error);
      }
    }
  }
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });
