import { task } from "hardhat/config";
import fs from "fs";
import path from "path";

// ERC20 Transfer 事件签名哈希 (keccak256("Transfer(address,address,uint256)"))
const ERC20_TRANSFER_SIGNATURE = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef";

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

task("scheduledTransfers", "执行定时转账任务（模拟每小时转账）")
  .addParam("rounds", "循环轮数", "5")
  .setAction(async (taskArgs, hre) => {
    const rounds = parseInt(taskArgs.rounds);
    const address = getContractAddress(hre.network.name);
    const myToken = await hre.ethers.getContractAt("MyToken", address);

    // 获取 signer
    const [deployer, user1, user2] = await hre.ethers.getSigners();
    const deployerAddr = await deployer.getAddress();
    const user1Addr = await user1.getAddress();
    const user2Addr = await user2.getAddress();

    console.log("=== 定时转账任务开始 ===");
    console.log("合约地址:", address);
    console.log("Deployer:", deployerAddr);
    console.log("User1:", user1Addr);
    console.log("User2:", user2Addr);
    console.log(`计划执行 ${rounds} 轮转账\n`);

    // ========== 初始铸造和分配 ==========
    console.log("========== 初始铸造和分配 ==========");

    // Deployer 铸造 300 ETH（包含自己和两个用户的份额）
    const ethAmount = hre.ethers.parseEther("300");
    console.log("Deployer 铸造 300 ETH...");
    const mintTx = await myToken.connect(deployer).mint({ value: ethAmount });
    await mintTx.wait();
    console.log("铸造完成, Hash:", mintTx.hash);

    // 给 User1 和 User2 各转 1000 token
    const userShare = hre.ethers.parseUnits("1000", 18);
    console.log("\nDeployer 向 User1 转账 1000 token...");
    const txUser1 = await myToken.connect(deployer).transfer(user1Addr, userShare);
    await txUser1.wait();

    console.log("Deployer 向 User2 转账 1000 token...");
    const txUser2 = await myToken.connect(deployer).transfer(user2Addr, userShare);
    await txUser2.wait();

    // 打印初始余额
    console.log("\n初始余额:");
    const deployerInitial = await myToken.balanceOf(deployerAddr);
    const user1Initial = await myToken.balanceOf(user1Addr);
    const user2Initial = await myToken.balanceOf(user2Addr);
    console.log("  Deployer:", hre.ethers.formatUnits(deployerInitial, 18));
    console.log("  User1:", hre.ethers.formatUnits(user1Initial, 18));
    console.log("  User2:", hre.ethers.formatUnits(user2Initial, 18));

    // ========== 定时转账循环 ==========
    for (let i = 0; i < rounds; i++) {
      console.log(`\n========== 第 ${i + 1} 轮转账 ==========`);
      console.log("当前时间:", new Date().toISOString());

      // 1. Deployer 向 user1 和 user2 各转 1000 token
      console.log("\n1. Deployer 向 User1 和 User2 各转 1000 token");
      const tx1 = await myToken.connect(deployer).transfer(user1Addr, hre.ethers.parseUnits("1000", 18));
      await tx1.wait();
      console.log("   -> User1 转账完成, Hash:", tx1.hash);

      const tx2 = await myToken.connect(deployer).transfer(user2Addr, hre.ethers.parseUnits("1000", 18));
      await tx2.wait();
      console.log("   -> User2 转账完成, Hash:", tx2.hash);

      // 2. User1 向 deployer 和 user2 各转 100 token
      console.log("\n2. User1 向 Deployer 和 User2 各转 100 token");
      const tx3 = await myToken.connect(user1).transfer(deployerAddr, hre.ethers.parseUnits("100", 18));
      await tx3.wait();
      console.log("   -> Deployer 转账完成, Hash:", tx3.hash);

      const tx4 = await myToken.connect(user1).transfer(user2Addr, hre.ethers.parseUnits("100", 18));
      await tx4.wait();
      console.log("   -> User2 转账完成, Hash:", tx4.hash);

      // 3. User2 连续向 deployer 和 user1 转账 10 次，每次 10 token
      console.log("\n3. User2 连续转账 10 次，每次向 Deployer 和 User1 各转 10 token");
      for (let j = 0; j < 10; j++) {
        const tx5 = await myToken.connect(user2).transfer(deployerAddr, hre.ethers.parseUnits("10", 18));
        await tx5.wait();

        const tx6 = await myToken.connect(user2).transfer(user1Addr, hre.ethers.parseUnits("10", 18));
        await tx6.wait();

        if (j === 0) console.log(`   -> 第 ${j + 1} 次完成`);
        if (j === 9) console.log(`   -> 第 ${j + 1} 次完成 (共10次)`);
      }

      // 打印当前余额
      console.log("\n当前余额:");
      const deployerBalance = await myToken.balanceOf(deployerAddr);
      const user1Balance = await myToken.balanceOf(user1Addr);
      const user2Balance = await myToken.balanceOf(user2Addr);

      console.log("  Deployer:", hre.ethers.formatUnits(deployerBalance, 18));
      console.log("  User1:", hre.ethers.formatUnits(user1Balance, 18));
      console.log("  User2:", hre.ethers.formatUnits(user2Balance, 18));

      // 如果不是最后一轮，增加 1 小时时间
      if (i < rounds - 1) {
        console.log("\n⏰ 增加 1 小时时间...");
        await hre.network.provider.send("evm_increaseTime", [3600]); // 3600 秒 = 1 小时
        await hre.network.provider.send("evm_mine"); // 挖一个新块

        // 获取当前区块时间
        const block = await hre.ethers.provider.getBlock("latest");
        console.log("新区块时间戳:", block?.timestamp,
          "(", new Date(Number(block?.timestamp) * 1000).toISOString(), ")");
      }
    }

    console.log("\n========== 所有转账完成 ==========");

    // 最终余额
    console.log("\n最终余额:");
    const finalDeployer = await myToken.balanceOf(deployerAddr);
    const finalUser1 = await myToken.balanceOf(user1Addr);
    const finalUser2 = await myToken.balanceOf(user2Addr);

    console.log("  Deployer:", hre.ethers.formatUnits(finalDeployer, 18));
    console.log("  User1:", hre.ethers.formatUnits(finalUser1, 18));
    console.log("  User2:", hre.ethers.formatUnits(finalUser2, 18));

    console.log("\n=== 定时转账任务结束 ===");
  });

task("scanBlocks", "扫描链上区块（支持范围查找和ERC20转账分析）")
  .addOptionalParam("from", "起始区块号", "")
  .addOptionalParam("to", "结束区块号", "")
  .addOptionalParam("analyze", "是否分析ERC20转账", "true")
  .setAction(async (taskArgs, hre) => {
    const provider = hre.ethers.provider;
    
    console.log("=== 扫描链上区块 ===");
    console.log("网络:", hre.network.name);
    
    const currentBlockNumber = await provider.getBlockNumber();
    console.log("当前区块高度:", currentBlockNumber);
    
    // 确定扫描范围
    let fromBlock: number;
    let toBlock: number;
    
    if (taskArgs.from && taskArgs.to) {
      fromBlock = parseInt(taskArgs.from);
      toBlock = parseInt(taskArgs.to);
    } else if (taskArgs.from) {
      fromBlock = parseInt(taskArgs.from);
      toBlock = fromBlock + 9; // 默认扫描10个区块
    } else {
      // 默认扫描最近10个区块
      toBlock = currentBlockNumber;
      fromBlock = Math.max(0, currentBlockNumber - 9);
    }
    
    // 确保范围有效
    fromBlock = Math.max(0, fromBlock);
    toBlock = Math.min(currentBlockNumber, toBlock);
    
    console.log(`扫描范围: Block #${fromBlock} 到 Block #${toBlock} (共 ${toBlock - fromBlock + 1} 个区块)\n`);
    
    const analyzeERC20 = taskArgs.analyze === "true";
    let totalERC20Transfers = 0;
    
    // 遍历区块
    for (let blockNum = fromBlock; blockNum <= toBlock; blockNum++) {
      const block = await provider.getBlock(blockNum);
      if (!block) continue;
      
      const timestamp = new Date(Number(block.timestamp) * 1000).toISOString();
      console.log(`\n📦 Block #${block.number} | ${timestamp} | 交易数: ${block.transactions.length}`);
      
      if (block.transactions.length === 0) {
        console.log("   (无交易)");
        continue;
      }
      
      // 分析每个交易
      for (let i = 0; i < block.transactions.length; i++) {
        const txHash = block.transactions[i];
        const tx = await provider.getTransaction(txHash);
        const receipt = await provider.getTransactionReceipt(txHash);
        
        if (!tx || !receipt) continue;
        
        // 显示基本交易信息
        const isContractCall = tx.data && tx.data !== "0x";
        const txType = isContractCall ? "📜 合约调用" : "💸 ETH转账";
        console.log(`\n   [${i + 1}] ${txType}`);
        console.log(`       Hash: ${tx.hash.substring(0, 30)}...`);
        console.log(`       From: ${tx.from}`);
        console.log(`       To: ${tx.to || "合约创建"}`);
        
        if (tx.value > 0) {
          console.log(`       Value: ${hre.ethers.formatEther(tx.value)} ETH`);
        }
        
        // 分析ERC20转账
        if (analyzeERC20 && receipt.logs.length > 0) {
          const erc20Transfers = receipt.logs.filter(log => 
            log.topics[0] === ERC20_TRANSFER_SIGNATURE
          );
          
          if (erc20Transfers.length > 0) {
            console.log(`       🪙 ERC20 Token 转账 (${erc20Transfers.length} 笔):`);
            
            for (let j = 0; j < erc20Transfers.length; j++) {
              const log = erc20Transfers[j];
              const tokenContract = log.address;
              
              // 解析转账事件参数
              // topics[1] = from (indexed), topics[2] = to (indexed), data = amount
              const from = "0x" + log.topics[1].substring(26);
              const to = "0x" + log.topics[2].substring(26);
              const amount = BigInt(log.data);
              
              console.log(`          [${j + 1}] Token: ${tokenContract.substring(0, 20)}...`);
              console.log(`              From: ${from}`);
              console.log(`              To: ${to}`);
              console.log(`              Amount: ${amount.toString()} (原始单位)`);
              
              totalERC20Transfers++;
            }
          }
        }
      }
    }
    
    console.log(`\n\n=== 扫描完成 ===`);
    console.log(`扫描区块: #${fromBlock} - #${toBlock}`);
    console.log(`总交易数: ${await getTotalTxCount(provider, fromBlock, toBlock)}`);
    if (analyzeERC20) {
      console.log(`ERC20转账: ${totalERC20Transfers} 笔`);
    }
  });

// 辅助函数：获取总交易数
async function getTotalTxCount(provider: any, fromBlock: number, toBlock: number): Promise<number> {
  let count = 0;
  for (let i = fromBlock; i <= toBlock; i++) {
    const block = await provider.getBlock(i);
    if (block) count += block.transactions.length;
  }
  return count;
}

task("getBlockTxs", "获取指定区块的所有交易")
  .addParam("block", "区块编号")
  .setAction(async (taskArgs, hre) => {
    const provider = hre.ethers.provider;
    const blockNumber = parseInt(taskArgs.block);
    
    console.log(`=== 获取区块 #${blockNumber} 的交易 ===`);
    
    const block = await provider.getBlock(blockNumber);
    
    if (!block) {
      console.log(`❌ 区块 #${blockNumber} 不存在`);
      return;
    }
    
    console.log("区块信息:");
    console.log("  区块号:", block.number);
    console.log("  区块哈希:", block.hash);
    console.log("  时间戳:", new Date(Number(block.timestamp) * 1000).toISOString());
    console.log("  交易数量:", block.transactions.length);
    console.log("  Gas Limit:", block.gasLimit.toString());
    console.log("  Gas Used:", block.gasUsed.toString());
    
    if (block.transactions.length === 0) {
      console.log("\n该区块没有交易");
    } else {
      console.log("\n交易列表:");
      
      for (let i = 0; i < block.transactions.length; i++) {
        const txHash = block.transactions[i];
        const tx = await provider.getTransaction(txHash);
        
        if (tx) {
          console.log(`\n  [${i + 1}] 交易哈希: ${tx.hash}`);
          console.log(`      From: ${tx.from}`);
          console.log(`      To: ${tx.to || "合约创建"}`);
          console.log(`      Value: ${hre.ethers.formatEther(tx.value)} ETH`);
          console.log(`      Gas Price: ${tx.gasPrice?.toString() || "N/A"}`);
          console.log(`      Nonce: ${tx.nonce}`);
          
          // 获取交易收据（包含 gas 使用情况）
          const receipt = await provider.getTransactionReceipt(txHash);
          if (receipt) {
            console.log(`      Gas Used: ${receipt.gasUsed.toString()}`);
            console.log(`      Status: ${receipt.status === 1 ? "✅ 成功" : "❌ 失败"}`);
          }
        }
      }
    }
    
    console.log("\n=== 获取完成 ===");
  });
