import { task } from "hardhat/config";
import fs from "fs";
import path from "path";

// 获取保存在本地的合约地址
function getContractAddress(networkName: string): string {
  const deploymentsPath = path.join(__dirname, "../deployments.json");
  if (!fs.existsSync(deploymentsPath)) {
    throw new Error("❌ deployments.json 文件不存在，请先执行部署脚本。");
  }
  const deployments = JSON.parse(fs.readFileSync(deploymentsPath, "utf8"));
  const address = deployments[networkName];
  if (!address) {
    throw new Error(`❌ 找不到在 ${networkName} 网络上的部署地址，请先部署。`);
  }
  return address;
}

task("mint", "支付 ETH 铸造代币")
  .addParam("eth", "支付的 ETH 数量 (例如 0.01)")
  .setAction(async (taskArgs, hre) => {
    const address = getContractAddress(hre.network.name);
    const myToken = await hre.ethers.getContractAt("MyToken", address);
    const ethAmount = hre.ethers.parseEther(taskArgs.eth);
    
    console.log(`正在支付 ${taskArgs.eth} ETH 铸造代币 (网络: ${hre.network.name})...`);
    const tx = await myToken.mint({ value: ethAmount });
    await tx.wait();
    console.log(`✅ 铸造成功! 交易 Hash: ${tx.hash}`);
  });

task("burn", "销毁当前账户的代币")
  .addParam("amount", "销毁的数量 (不带精度)")
  .setAction(async (taskArgs, hre) => {
    const address = getContractAddress(hre.network.name);
    const myToken = await hre.ethers.getContractAt("MyToken", address);
    const amount = hre.ethers.parseUnits(taskArgs.amount, 18);
    
    console.log(`正在销毁 ${taskArgs.amount} 代币 (网络: ${hre.network.name})...`);
    const tx = await myToken.burn(amount);
    await tx.wait();
    console.log(`✅ 销毁成功! 交易 Hash: ${tx.hash}`);
  });

task("batchTransfer", "批量向多个地址转账代币")
  .addParam("recipients", "接收者地址列表，使用逗号分隔")
  .addParam("amounts", "每个地址对应的代币数量，使用逗号分隔")
  .setAction(async (taskArgs, hre) => {
    const address = getContractAddress(hre.network.name);
    const myToken = await hre.ethers.getContractAt("MyToken", address);
    
    const recipients = taskArgs.recipients.split(",").map((a: string) => a.trim());
    const amounts = taskArgs.amounts.split(",").map((a: string) => hre.ethers.parseUnits(a.trim(), 18));
    
    if (recipients.length !== amounts.length) {
      throw new Error("❌ 错误：接收者数量和金额数量不一致！");
    }

    console.log(`正在批量转账，涉及 ${recipients.length} 个地址 (网络: ${hre.network.name})...`);
    const tx = await myToken.batchTransfer(recipients, amounts);
    await tx.wait();
    console.log(`✅ 批量转账成功! 交易 Hash: ${tx.hash}`);
  });

task("withdrawETH", "提取合约中的 ETH")
  .setAction(async (taskArgs, hre) => {
    const address = getContractAddress(hre.network.name);
    const myToken = await hre.ethers.getContractAt("MyToken", address);
    
    console.log(`正在提取合约中的 ETH (网络: ${hre.network.name})...`);
    const tx = await myToken.withdrawETH();
    await tx.wait();
    console.log(`✅ 提取成功! 交易 Hash: ${tx.hash}`);
  });
