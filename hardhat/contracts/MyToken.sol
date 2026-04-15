// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "hardhat/console.sol";

contract MyToken is ERC20, Ownable {
    uint256 public constant RATE = 10000; // 10000 MyToken per 1 ETH    
    uint256 public constant MIN_ETH = 0.001 ether;

    constructor(address initialOwner) ERC20("RDemoToken", "RDT") Ownable(msg.sender) {
    }

    // 只允许通过支付 ETH 来铸造 token
    function mint() public payable {
        require(msg.value >= MIN_ETH, "Not enough ETH sent");
        uint256 tokensToMint = (msg.value * RATE);
        console.log("tokensToMint:", tokensToMint);
        _mint(msg.sender, tokensToMint);
    }

    // 允许用户销毁自己的Token
    function burn(uint256 amount) public {
        _burn(msg.sender, amount);
    }

    // 批量转账功能 (任何用户都可以调用，只要余额足够)
    function batchTransfer(address[] calldata recipients, uint256[] calldata amounts) public {
        require(recipients.length == amounts.length, "Arrays length mismatch");
        for (uint256 i = 0; i < recipients.length; i++) {
            transfer(recipients[i], amounts[i]);
        }
    }

    function withdrawETH() public onlyOwner {
        uint256 balance = address(this).balance;
        require(balance > 0, "No ETH to withdraw");
        payable(owner()).transfer(balance);
    }

    receive() external payable {
        mint();
    }
}
