package ethereum

import (
	"context"
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/models"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// TokenIndexer handles the indexing of token transfer events
type TokenIndexer struct {
	client      *ethclient.Client
	tokenClient *TokenClient
	config      *config.Config
	lastBlock   int64
}

// NewTokenIndexer creates a new TokenIndexer instance
func NewTokenIndexer(cfg *config.Config) (*TokenIndexer, error) {
	// Connect to Ethereum client
	client, err := ethclient.Dial(cfg.EthNodeURL)
	if err != nil {
		return nil, err
	}

	// Create token client
	tokenAddr := common.HexToAddress(cfg.TokenContractAddress)
	tokenClient, err := NewTokenClient(client, tokenAddr)
	if err != nil {
		return nil, err
	}

	// Initialize lastBlock to startBlockNumber (default fallback)
	lastBlock := cfg.StartBlockNumber

	// Try to get contract deployment block
	deploymentBlock, err := tokenClient.GetContractDeploymentBlock(context.Background())
	if err == nil {
		logger.Log.Infof("Found contract deployment block: %d", deploymentBlock.Int64())
		// Use deployment block as the start block if it's later than config value
		if deploymentBlock.Int64() > lastBlock {
			lastBlock = deploymentBlock.Int64()
		}
	} else {
		logger.Log.Warnf("Failed to get contract deployment block, using config value: %v", err)
	}

	// Check if we have a stored lastBlock in the database
	var balance models.Balance
	if err := database.DB.Order("last_updated_block desc").First(&balance).Error; err == nil {
		// Use the database stored block if it's later than current value
		if balance.LastUpdatedBlock+1 > lastBlock {
			lastBlock = balance.LastUpdatedBlock + 1
		}
	}

	return &TokenIndexer{
		client:      client,
		tokenClient: tokenClient,
		config:      cfg,
		lastBlock:   lastBlock,
	}, nil
}

// Start begins the indexing process
func (ti *TokenIndexer) Start() {
	logger.Log.Infof("Starting token indexer, polling every %d seconds from block %d", ti.config.PollingInterval, ti.lastBlock)

	// Run immediately
	ti.goRunIndexing()

	// Set up the cron job
	ticker := time.NewTicker(time.Duration(ti.config.PollingInterval) * time.Second)
	go func() {
		for range ticker.C {
			ti.goRunIndexing()
		}
	}()
}

// goRunIndexing runs the indexing process in a goroutine
func (ti *TokenIndexer) goRunIndexing() {
	go func() {
		if err := ti.runIndexing(); err != nil {
			logger.Log.Errorf("Error running indexing: %v", err)
		}
	}()
}

// runIndexing performs the actual indexing process
func (ti *TokenIndexer) runIndexing() error {
	ctx := context.Background()

	// Get the latest block number
	latestBlock, err := ti.client.BlockNumber(ctx)
	if err != nil {
		return err
	}

	// Calculate end block
	endBlock := ti.lastBlock + int64(ti.config.BlockBatchSize)
	if endBlock > int64(latestBlock) {
		endBlock = int64(latestBlock)
	}

	// If we're already at or past the latest block, do nothing
	if ti.lastBlock > endBlock {
		return nil
	}

	logger.Log.Infof("Indexing blocks %d to %d", ti.lastBlock, endBlock)

	// Fetch transfer events
	transfers, err := ti.tokenClient.GetTransferEvents(
		ctx,
		big.NewInt(ti.lastBlock),
		big.NewInt(endBlock),
	)
	if err != nil {
		return err
	}

	// Process transfers  models.Transfer 的构建并不容易
	for _, transfer := range transfers {
		// Save transfer to database
		transferModel := models.Transfer{
			BlockNum: int64(transfer.BlockNum),
			TxHash:   transfer.TxHash.Hex(),
			LogIdx:   int(transfer.LogIndex),
			FromAddr: transfer.From.Hex(),
			ToAddr:   transfer.To.Hex(),
			Amount:   transfer.Amount.String(),
		}

		if err := database.DB.Create(&transferModel).Error; err != nil {
			logger.Log.Errorf("Error saving transfer: %v", err)
			continue
		}

		// Update balances
		ti.updateBalance(transfer.From.Hex(), "-", transfer.Amount)
		ti.updateBalance(transfer.To.Hex(), "+", transfer.Amount)
	}

	// Update lastBlock
	ti.lastBlock = endBlock + 1

	logger.Log.Infof("Indexed %d transfers from blocks %d to %d", len(transfers), ti.lastBlock-int64(ti.config.BlockBatchSize), endBlock)

	return nil
}

// updateBalance updates the balance of an address in the database
func (ti *TokenIndexer) updateBalance(address, operation string, amount *big.Int) {
	var balance models.Balance

	// Find existing balance or initialize a new one
	database.DB.Where("address = ?", address).FirstOrInit(&balance)
	// Find existing balance
	// result := database.DB.Where("address = ?", address).First(&balance)
	// Parse current balance
	currentBalance := big.NewInt(0)
	if balance.Balance != "" {
		currentBalance, _ = new(big.Int).SetString(balance.Balance, 10)
	}

	// Calculate new balance
	newBalance := big.NewInt(0)
	switch operation {
	case "+":
		newBalance.Add(currentBalance, amount)
	case "-":
		newBalance.Sub(currentBalance, amount)
		// Ensure balance doesn't go negative
		if newBalance.Sign() < 0 {
			newBalance.SetInt64(0)
		}
	}

	// Prepare update data
	updateData := map[string]interface{}{
		"balance":            newBalance.String(),
		"last_updated_block": ti.lastBlock - 1,
	}

	// Upsert balance
	if balance.Address == "" {
		// Create new balance
		balance.Address = address
		balance.Balance = newBalance.String()
		balance.LastUpdatedBlock = ti.lastBlock - 1
		if err := database.DB.Create(&balance).Error; err != nil {
			logger.Log.Errorf("Error creating balance for %s: %v", address, err)
		}
	} else {
		// Update existing balance
		if err := database.DB.Model(&balance).Updates(updateData).Error; err != nil {
			logger.Log.Errorf("Error updating balance for %s: %v", address, err)
		}
	}
}
