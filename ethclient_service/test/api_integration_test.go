package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

// getBaseURL 获取服务基础 URL
func getBaseURL() string {
	// 尝试从环境变量获取
	if url := os.Getenv("API_BASE_URL"); url != "" {
		return url
	}
	// 默认使用本地服务
	return "http://localhost:8080"
}

// makeRequest 发送 HTTP 请求
func makeRequest(t *testing.T, method, path string, body interface{}) (*http.Response, string) {
	loadEnv(t)
	baseURL := getBaseURL()
	url := baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		bodyReader = bytes.NewBuffer(bodyBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return resp, string(respBody)
}

// getFirstChainID 获取数据库中第一个链的 chain_id
func getFirstChainID(t *testing.T) string {
	resp, body := makeRequest(t, "GET", "/api/chains", nil)
	if resp.StatusCode != 200 {
		t.Logf("获取链列表失败，状态码: %d", resp.StatusCode)
		return ""
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Logf("解析链列表失败: %v", err)
		return ""
	}

	chains, ok := result["chains"].([]interface{})
	if !ok || len(chains) == 0 {
		t.Log("数据库中没有链记录")
		return ""
	}

	firstChain, ok := chains[0].(map[string]interface{})
	if !ok {
		t.Log("解析链数据失败")
		return ""
	}

	chainID, ok := firstChain["chain_id"].(string)
	if !ok {
		t.Log("获取 chain_id 失败")
		return ""
	}

	return chainID
}

// TestHealthAPI 测试健康检查 API
func TestHealthAPI(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/health", nil)
	t.Logf("Health API 响应状态码: %d", resp.StatusCode)
	t.Logf("Health API 响应体: %s", body)
}

// TestChainAPI_GetAllChains 测试获取所有链信息 API
// GET /api/chains - 不传任何参数
func TestChainAPI_GetAllChains(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/api/chains", nil)
	t.Logf("GetAllChains API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetAllChains API 响应体: %s", body)
}

// TestChainAPI_GetChainConfigs 测试获取链配置 API
func TestChainAPI_GetChainConfigs(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/api/chains/configs", nil)
	t.Logf("GetChainConfigs API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetChainConfigs API 响应体: %s", body)
}

// TestChainAPI_GetPointsRate 测试获取积分比率 API
func TestChainAPI_GetPointsRate(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/api/chains/points-rate", nil)
	t.Logf("GetPointsRate API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsRate API 响应体: %s", body)
}

// TestChainAPI_UpdatePointsRate 测试更新积分比率 API
func TestChainAPI_UpdatePointsRate(t *testing.T) {
	updateBody := map[string]float64{
		"rate": 0.1,
	}
	resp, body := makeRequest(t, "POST", "/api/chains/points-rate", updateBody)
	t.Logf("UpdatePointsRate API 响应状态码: %d", resp.StatusCode)
	t.Logf("UpdatePointsRate API 响应体: %s", body)
}

// TestChainAPI_GetChainByID 测试获取单个链信息 API
// 先获取所有链，然后使用第一个链的 ID 进行测试
func TestChainAPI_GetChainByID(t *testing.T) {
	// 先获取所有链
	resp, body := makeRequest(t, "GET", "/api/chains", nil)
	if resp.StatusCode != 200 {
		t.Logf("获取链列表失败，状态码: %d", resp.StatusCode)
		return
	}

	// 解析响应获取链 ID
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Logf("解析链列表失败: %v", err)
		return
	}

	chains, ok := result["chains"].([]interface{})
	if !ok || len(chains) == 0 {
		t.Log("数据库中没有链记录，跳过测试")
		return
	}

	// 获取第一个链的 ID (注意：这里用 "id" 不是 "chain_id")
	firstChain, ok := chains[0].(map[string]interface{})
	if !ok {
		t.Log("解析链数据失败")
		return
	}

	id, ok := firstChain["id"].(string)
	if !ok {
		t.Log("获取链 ID 失败")
		return
	}

	t.Logf("使用链 ID: %s 进行测试", id)

	resp, body = makeRequest(t, "GET", fmt.Sprintf("/api/chains/%s", id), nil)
	t.Logf("GetChainByID API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetChainByID API 响应体: %s", body)
}

// TestChainAPI_SyncChain 测试同步链 API
// 先获取所有链，然后使用第一个链的 ID 进行测试
func TestChainAPI_SyncChain(t *testing.T) {
	// 先获取所有链
	resp, body := makeRequest(t, "GET", "/api/chains", nil)
	if resp.StatusCode != 200 {
		t.Logf("获取链列表失败，状态码: %d", resp.StatusCode)
		return
	}

	// 解析响应获取链 ID
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Logf("解析链列表失败: %v", err)
		return
	}

	chains, ok := result["chains"].([]interface{})
	if !ok || len(chains) == 0 {
		t.Log("数据库中没有链记录，跳过测试")
		return
	}

	// 获取第一个链的 ID (注意：这里用 "id" 不是 "chain_id")
	firstChain, ok := chains[0].(map[string]interface{})
	if !ok {
		t.Log("解析链数据失败")
		return
	}

	id, ok := firstChain["id"].(string)
	if !ok {
		t.Log("获取链 ID 失败")
		return
	}

	t.Logf("使用链 ID: %s 进行测试", id)

	syncBody := map[string]int64{
		"from_block": 1000000,
	}
	resp, body = makeRequest(t, "POST", fmt.Sprintf("/api/chains/%s/sync", id), syncBody)
	t.Logf("SyncChain API 响应状态码: %d", resp.StatusCode)
	t.Logf("SyncChain API 响应体: %s", body)
}

// TestBalanceAPI_GetAllBalances 测试获取所有余额 API
func TestBalanceAPI_GetAllBalances(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/api/balances", nil)
	t.Logf("GetAllBalances API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetAllBalances API 响应体: %s", body)
}

// TestBalanceAPI_GetAllBalancesWithFilters 测试获取所有余额 API（带过滤参数）
// 先获取所有链，然后使用第一个链的 chain_id 进行测试
func TestBalanceAPI_GetAllBalancesWithFilters(t *testing.T) {
	chainID := getFirstChainID(t)
	if chainID == "" {
		t.Log("获取 chain_id 失败，使用默认参数测试")
		resp, body := makeRequest(t, "GET", "/api/balances?chain_id=sepolia-001&min_balance=100&page=1&limit=20", nil)
		t.Logf("GetAllBalancesWithFilters API 响应状态码: %d", resp.StatusCode)
		t.Logf("GetAllBalancesWithFilters API 响应体: %s", body)
		return
	}

	t.Logf("使用 chain_id: %s 进行测试", chainID)

	// 先不带 min_balance 查询，看看是否有数据
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/balances?chain_id=%s&page=1&limit=20", chainID), nil)
	t.Logf("GetAllBalancesWithFilters API (无min_balance) 响应状态码: %d", resp.StatusCode)
	t.Logf("GetAllBalancesWithFilters API (无min_balance) 响应体: %s", body)

	// 再带 min_balance 查询
	resp, body = makeRequest(t, "GET", fmt.Sprintf("/api/balances?chain_id=%s&min_balance=100&page=1&limit=20", chainID), nil)
	t.Logf("GetAllBalancesWithFilters API (min_balance=100) 响应状态码: %d", resp.StatusCode)
	t.Logf("GetAllBalancesWithFilters API (min_balance=100) 响应体: %s", body)
}

// TestBalanceAPI_GetUserBalance 测试获取用户余额 API
func TestBalanceAPI_GetUserBalance(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/balances/%s", testAddress), nil)
	t.Logf("GetUserBalance API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetUserBalance API 响应体: %s", body)
}

// TestBalanceAPI_GetUserBalanceWithChainID 测试获取用户余额 API（带链ID参数）
// 先获取所有链，然后使用第一个链的 chain_id 进行测试
func TestBalanceAPI_GetUserBalanceWithChainID(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"

	chainID := getFirstChainID(t)
	if chainID == "" {
		t.Log("获取 chain_id 失败，使用默认参数测试")
		resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/balances/%s?chain_id=sepolia-001", testAddress), nil)
		t.Logf("GetUserBalanceWithChainID API 响应状态码: %d", resp.StatusCode)
		t.Logf("GetUserBalanceWithChainID API 响应体: %s", body)
		return
	}

	t.Logf("使用 chain_id: %s 进行测试", chainID)

	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/balances/%s?chain_id=%s", testAddress, chainID), nil)
	t.Logf("GetUserBalanceWithChainID API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetUserBalanceWithChainID API 响应体: %s", body)
}

// TestBalanceAPI_GetBalanceHistory 测试获取余额历史 API
func TestBalanceAPI_GetBalanceHistory(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/balances/%s/history", testAddress), nil)
	t.Logf("GetBalanceHistory API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetBalanceHistory API 响应体: %s", body)
}

// TestBalanceAPI_GetBalanceHistoryWithPagination 测试获取余额历史 API（带分页参数）
func TestBalanceAPI_GetBalanceHistoryWithPagination(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/balances/%s/history?page=1&limit=10", testAddress), nil)
	t.Logf("GetBalanceHistoryWithPagination API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetBalanceHistoryWithPagination API 响应体: %s", body)
}

// TestBalanceAPI_RebuildUserBalance 测试重建用户余额 API
func TestBalanceAPI_RebuildUserBalance(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	rebuildBody := map[string]string{
		"chain_id": "sepolia-001",
	}
	resp, body := makeRequest(t, "POST", fmt.Sprintf("/api/balances/%s/rebuild", testAddress), rebuildBody)
	t.Logf("RebuildUserBalance API 响应状态码: %d", resp.StatusCode)
	t.Logf("RebuildUserBalance API 响应体: %s", body)
}

// TestPointsAPI_GetPointsLeaderboard 测试获取积分排行榜 API
func TestPointsAPI_GetPointsLeaderboard(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/api/points", nil)
	t.Logf("GetPointsLeaderboard API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsLeaderboard API 响应体: %s", body)
}

// TestPointsAPI_GetPointsLeaderboardWithFilters 测试获取积分排行榜 API（带过滤参数）
// 先获取所有链，然后使用第一个链的 chain_id 进行测试
func TestPointsAPI_GetPointsLeaderboardWithFilters(t *testing.T) {
	chainID := getFirstChainID(t)
	if chainID == "" {
		t.Log("获取 chain_id 失败，使用默认参数测试")
		resp, body := makeRequest(t, "GET", "/api/points?chain_id=sepolia-001&page=1&limit=50", nil)
		t.Logf("GetPointsLeaderboardWithFilters API 响应状态码: %d", resp.StatusCode)
		t.Logf("GetPointsLeaderboardWithFilters API 响应体: %s", body)
		return
	}

	t.Logf("使用 chain_id: %s 进行测试", chainID)

	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points?chain_id=%s&page=1&limit=50", chainID), nil)
	t.Logf("GetPointsLeaderboardWithFilters API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsLeaderboardWithFilters API 响应体: %s", body)
}

// TestPointsAPI_GetPointsStats 测试获取积分统计 API
func TestPointsAPI_GetPointsStats(t *testing.T) {
	resp, body := makeRequest(t, "GET", "/api/points/stats", nil)
	t.Logf("GetPointsStats API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsStats API 响应体: %s", body)
}

// TestPointsAPI_GetPointsStatsWithChainID 测试获取积分统计 API（带链ID参数）
// 先获取所有链，然后使用第一个链的 chain_id 进行测试
func TestPointsAPI_GetPointsStatsWithChainID(t *testing.T) {
	chainID := getFirstChainID(t)
	if chainID == "" {
		t.Log("获取 chain_id 失败，使用默认参数测试")
		resp, body := makeRequest(t, "GET", "/api/points/stats?chain_id=sepolia-001", nil)
		t.Logf("GetPointsStatsWithChainID API 响应状态码: %d", resp.StatusCode)
		t.Logf("GetPointsStatsWithChainID API 响应体: %s", body)
		return
	}

	t.Logf("使用 chain_id: %s 进行测试", chainID)

	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points/stats?chain_id=%s", chainID), nil)
	t.Logf("GetPointsStatsWithChainID API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsStatsWithChainID API 响应体: %s", body)
}

// TestPointsAPI_GetUserPoints 测试获取用户积分 API
func TestPointsAPI_GetUserPoints(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points/%s", testAddress), nil)
	t.Logf("GetUserPoints API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetUserPoints API 响应体: %s", body)
}

// TestPointsAPI_GetUserPointsWithChainID 测试获取用户积分 API（带链ID参数）
// 先获取所有链，然后使用第一个链的 chain_id 进行测试
func TestPointsAPI_GetUserPointsWithChainID(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"

	chainID := getFirstChainID(t)
	if chainID == "" {
		t.Log("获取 chain_id 失败，使用默认参数测试")
		resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points/%s?chain_id=sepolia-001", testAddress), nil)
		t.Logf("GetUserPointsWithChainID API 响应状态码: %d", resp.StatusCode)
		t.Logf("GetUserPointsWithChainID API 响应体: %s", body)
		return
	}

	t.Logf("使用 chain_id: %s 进行测试", chainID)

	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points/%s?chain_id=%s", testAddress, chainID), nil)
	t.Logf("GetUserPointsWithChainID API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetUserPointsWithChainID API 响应体: %s", body)
}

// TestPointsAPI_GetPointsHistory 测试获取积分历史 API
func TestPointsAPI_GetPointsHistory(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points/%s/history", testAddress), nil)
	t.Logf("GetPointsHistory API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsHistory API 响应体: %s", body)
}

// TestPointsAPI_GetPointsHistoryWithPagination 测试获取积分历史 API（带分页参数）
func TestPointsAPI_GetPointsHistoryWithPagination(t *testing.T) {
	testAddress := "0x40eAE793f36076c377435e66903950Cb8293Eb50"
	resp, body := makeRequest(t, "GET", fmt.Sprintf("/api/points/%s/history?page=1&limit=20", testAddress), nil)
	t.Logf("GetPointsHistoryWithPagination API 响应状态码: %d", resp.StatusCode)
	t.Logf("GetPointsHistoryWithPagination API 响应体: %s", body)
}

// TestPointsAPI_TriggerCalculation 测试触发积分计算 API
func TestPointsAPI_TriggerCalculation(t *testing.T) {
	calcBody := map[string]string{
		"address":  "0x40eAE793f36076c377435e66903950Cb8293Eb50",
		"chain_id": "sepolia-001",
	}
	resp, body := makeRequest(t, "POST", "/api/points/calculate", calcBody)
	t.Logf("TriggerCalculation API 响应状态码: %d", resp.StatusCode)
	t.Logf("TriggerCalculation API 响应体: %s", body)
}
