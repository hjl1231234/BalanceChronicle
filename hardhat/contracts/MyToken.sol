// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "hardhat/console.sol";

contract MyToken is ERC20, Ownable {
    uint256 public constant RATE = 100000000; // 100000000 MyToken per 1 ETH
    uint256 public constant MIN_ETH = 0.001 ether;

    constructor(address initialOwner) ERC20("RDemoToken", "RDT") Ownable(msg.sender) {
    }

    function mint() public payable {
        require(msg.value >= MIN_ETH, "Not enough ETH sent");
        uint256 tokensToMint = (msg.value * RATE);
        console.log("tokensToMint:", tokensToMint);
        _mint(msg.sender, tokensToMint);
    }

    // --- 新增：支持直接铸造、销毁和批量操作 ---

    // 允许拥有者给指定地址铸造Token (不需要消耗ETH)
    function mintTo(address to, uint256 amount) public onlyOwner {
        _mint(to, amount);
    }

    // 允许用户销毁自己的Token
    function burn(uint256 amount) public {
        _burn(msg.sender, amount);
    }

    // 允许拥有者批量铸造 (给多个地址发放相同或不同的数量)
    function batchMint(address[] calldata recipients, uint256[] calldata amounts) public onlyOwner {
        require(recipients.length == amounts.length, "Arrays length mismatch");
        for (uint256 i = 0; i < recipients.length; i++) {
            _mint(recipients[i], amounts[i]);
        }
    }

    // 批量转账功能 (任何用户都可以调用，只要余额足够)
    function batchTransfer(address[] calldata recipients, uint256[] calldata amounts) public {
        require(recipients.length == amounts.length, "Arrays length mismatch");
        for (uint256 i = 0; i < recipients.length; i++) {
            transfer(recipients[i], amounts[i]);
        }
    }

    // 批量销毁功能 (销毁自己账户下的token)
    // 实际上通常不需要 batchBurn，因为一次调 burn(总额) 即可。如果需要销毁多个账户的，
    // 需要通过 burnFrom 且前提是获得 allowance，这里提供一个简单的 burnFrom 包装。
    function burnFrom(address account, uint256 amount) public {
        uint256 currentAllowance = allowance(account, msg.sender);
        require(currentAllowance >= amount, "ERC20: burn amount exceeds allowance");
        _approve(account, msg.sender, currentAllowance - amount);
        _burn(account, amount);
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
