// ✅ 修改后（Sepolia）
import { HardhatUserConfig } from "hardhat/config";
import "@nomicfoundation/hardhat-toolbox";
import "dotenv/config";
import "@openzeppelin/hardhat-upgrades";
import "./tasks/myTokenTasks"; // 引入自定义的 Tasks
import "./tasks/scheduledTransfers"; // 引入定时转账任务

const config: HardhatUserConfig = {
  solidity: {
    version: "0.8.22",
    settings: {
      optimizer: {
        enabled: false,
        runs: 9999,
      },
    },
  },
  networks: {
        sepolia: {
      url: `https://sepolia.infura.io/v3/${process.env.INFURA_KEY}`,
      accounts: [
        process.env.PRIVATE_KEY731 || "",
        process.env.PRIVATE_KEY137 || "",
        process.env.PRIVATE_KEYf07d || ""
      ].filter(k => k !== "")
    },
    localhost: {
      url: "http://127.0.0.1:8545" // 连接到通过 npx hardhat node 启动的本地节点
            ,chainId: 31337,
      // mining: {
      //   auto: true,
      //   interval: 2000,
      // },
    },
  },
  etherscan: {
    apiKey: process.env.API_KEY,
  },
  // 可选：启用 Sourcify 验证
  sourcify: {
    enabled: false
  }
};

export default config;
