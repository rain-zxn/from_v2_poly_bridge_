/*
 * Copyright (C) 2020 The poly network Authors
 * This file is part of The poly network library.
 *
 * The  poly network  is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The  poly network  is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 * You should have received a copy of the GNU Lesser General Public License
 * along with The poly network .  If not, see <http://www.gnu.org/licenses/>.
 */

package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"poly-bridge/chainsdk"
	"poly-bridge/conf"
	"poly-bridge/models"
	"poly-bridge/nft_http/meta"
	"regexp"
	"strings"
	"time"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db           *gorm.DB
	chainConfig  = make(map[uint64]*conf.ChainListenConfig)
	txCounter    *TransactionCounter
	sdks         = make(map[uint64]*chainsdk.EthereumSdkPro)
	assets       = make([]*models.Token, 0)
	wrapperAddrs = make(map[uint64]common.Address)
	fetcher      *meta.StoreFetcher
	feeTokens    = make(map[uint64]*models.Token)
	lruDB        *lru.ARCCache
)

func NewDB(cfg *conf.DBConfig) *gorm.DB {
	user := cfg.User
	password := cfg.Password
	url := cfg.URL
	scheme := cfg.Scheme
	Logger := logger.Default
	if cfg.Debug {
		Logger = Logger.LogMode(logger.Info)
	}
	format := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8", user, password, url, scheme)
	db, err := gorm.Open(mysql.Open(format), &gorm.Config{Logger: Logger})
	if err != nil {
		panic(err)
	}

	db.Where("standard = ? and property=?", models.TokenTypeErc721, 1).
		Preload("TokenBasic").
		Find(&assets)
	for _, v := range assets {
		logs.Info("load asset %s, chainid %d, hash %s", v.TokenBasicName, v.ChainId, v.Hash)
	}

	feeTokenList := make([]*models.Token, 0)
	db.Where("hash=?", nativeHash).
		Preload("TokenBasic").
		Find(&feeTokenList)
	for _, v := range feeTokenList {
		feeTokens[v.ChainId] = v
		logs.Info("load chainid %d feeToken %s", v.ChainId, v.TokenBasicName)
	}
	return db
}

func Initialize(c *conf.Config) {
	for _, v := range c.ChainListenConfig {
		chainConfig[v.ChainId] = v
	}

	db = NewDB(c.DBConfig)

	arcLRU, err := lru.NewARC(5000)
	if err != nil {
		panic(err)
	}
	lruDB = arcLRU

	fetcher = meta.NewStoreFetcher(db)
	for _, asset := range assets {
		if asset.TokenBasic == nil {
			continue
		}
		fetcherTyp := meta.FetcherType(asset.TokenBasic.MetaFetcherType)
		baseUri := asset.TokenBasic.Meta
		assetName := asset.TokenBasic.Name
		fetcher.Register(fetcherTyp, assetName, baseUri)
	}

	txCounter = NewTransactionCounter()
}

type TransactionCounter struct {
	Count    int64
	LastTime int64
}

func NewTransactionCounter() *TransactionCounter {
	s := new(TransactionCounter)
	s.refresh()
	return s
}

func (s *TransactionCounter) refresh() {
	db.Model(&models.WrapperTransaction{}).
		Where("standard = ?", models.TokenTypeErc721).
		Count(&s.Count)

	s.LastTime = time.Now().Unix()
}

func (s *TransactionCounter) Number() int64 {
	now := time.Now().Unix()
	if now-s.LastTime < 120 {
		return s.Count
	}

	s.refresh()
	return s.Count
}

func selectNodeAndWrapper(chainId uint64) (*chainsdk.EthereumSdkPro, common.Address, error) {
	cfg, ok := chainConfig[chainId]
	if !ok {
		return nil, emptyAddr, fmt.Errorf("chain id %d invalid", chainId)
	}

	pro, ok := sdks[chainId]
	if !ok {
		urls := cfg.GetNodesUrl()
		if len(urls) == 0 {
			return nil, emptyAddr, fmt.Errorf("chainId %d not exist", chainId)
		}
		pro = chainsdk.NewEthereumSdkPro(urls, cfg.ListenSlot, chainId)
		sdks[chainId] = pro
	}

	wrapper, ok := wrapperAddrs[chainId]
	if !ok {
		wrapper = common.HexToAddress(cfg.NFTWrapperContract)
		wrapperAddrs[chainId] = wrapper
	}
	return pro, wrapper, nil
}

var emptyAddr = common.Address{}

func selectNFTAsset(addr string) *models.Token {
	for _, v := range assets {
		origin := common.HexToAddress(v.Hash)
		src := common.HexToAddress(addr)
		if bytes.Equal(origin.Bytes(), src.Bytes()) {
			return v
		}
	}
	return nil
}

func selectAssetsByChainId(chainId uint64) []*models.Token {
	res := make([]*models.Token, 0)
	for _, v := range assets {
		if v.ChainId == chainId {
			res = append(res, v)
		}
	}
	return res
}

const (
	ErrCodeRequest     int = 400
	ErrCodeNotExist    int = 404
	ErrCodeNodeInvalid int = 500
)

var errMap = map[int]string{
	ErrCodeRequest:     "request parameter is invalid!",
	ErrCodeNotExist:    "not found",
	ErrCodeNodeInvalid: "blockchain node exception",
}

func input(c *beego.Controller, req interface{}) bool {
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		code := ErrCodeRequest
		customInput(c, code, errMap[code])
		return false
	} else {
		return true
	}
}

func customInput(c *beego.Controller, code int, msg string) {
	c.Data["json"] = models.MakeErrorRsp(msg)
	c.Ctx.ResponseWriter.WriteHeader(code)
	c.ServeJSON()
}

func notExist(c *beego.Controller) {
	code := ErrCodeNotExist
	c.Data["json"] = models.MakeErrorRsp(errMap[code])
	c.Ctx.ResponseWriter.WriteHeader(code)
	c.ServeJSON()
}

func checkPageSize(c *beego.Controller, size int) bool {
	if size <= 10 {
		return true
	}
	code := ErrCodeRequest
	c.Data["json"] = models.MakeErrorRsp("page size too big, should be smaller than 10")
	c.Ctx.ResponseWriter.WriteHeader(code)
	c.ServeJSON()
	return false
}

func checkNumString(numStr string) (*big.Int, error) {
	numStr = strings.Trim(numStr, " ")
	if numStr == "" {
		return nil, fmt.Errorf("number string is empty")
	}

	ok, err := regexp.Match(`^\d+$`, []byte(numStr))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invalid number string")
	}
	data, ok := new(big.Int).SetString(numStr, 10)
	if !ok {
		return nil, fmt.Errorf("convert string to big int err")
	}
	return data, nil
}

func nodeInvalid(c *beego.Controller) {
	code := ErrCodeNodeInvalid
	c.Data["json"] = models.MakeErrorRsp(errMap[code])
	c.Ctx.ResponseWriter.WriteHeader(code)
	c.ServeJSON()
}

func output(c *beego.Controller, data interface{}) {
	c.Data["json"] = data
	c.ServeJSON()
}

func customOutput(c *beego.Controller, code int, msg string) {
	c.Data["json"] = models.MakeErrorRsp(msg)
	c.Ctx.ResponseWriter.WriteHeader(code)
	c.ServeJSON()
}

func getPageNo(totalNo, pageSize int) int {
	return (int(totalNo) + pageSize - 1) / pageSize
}

func findFeeToken(cid uint64, hash string) *models.Token {
	feeTokens := make([]*models.Token, 0)
	db.Model(&models.Token{}).
		Where("hash = ?", nativeHash).
		Preload("TokenBasic").
		Find(&feeTokens)

	for _, v := range feeTokens {
		if cid == v.ChainId && hash == v.Hash {
			return v
		}
	}
	return nil
}

func findAsset(cid uint64, hash string) *models.Token {
	for _, v := range assets {
		if v.ChainId == cid && hash == v.Hash {
			return v
		}
	}
	return nil
}
