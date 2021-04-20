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
	"fmt"
	"math/big"
	"poly-bridge/chainsdk"
	"poly-bridge/models"
	mcm "poly-bridge/nft_http/meta/common"
	"sort"
	"time"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
	"github.com/ethereum/go-ethereum/common"
)

type ItemController struct {
	beego.Controller
}

func (c *ItemController) Items() {
	var req ItemsOfAddressReq
	if !input(&c.Controller, &req) {
		return
	}

	if req.TokenId != "" {
		c.fetchSingleNFTItem(&req)
	} else {
		c.batchFetchNFTItems(&req)
	}
}

func (c *ItemController) fetchSingleNFTItem(req *ItemsOfAddressReq) {

	sdk := selectNode(req.ChainId)
	if sdk == nil {
		customInput(&c.Controller, ErrCodeRequest, "chain id not exist")
		return
	}
	wrapper := selectWrapper(req.ChainId)
	if wrapper == emptyAddr {
		customInput(&c.Controller, ErrCodeRequest, "chain id not exist")
		return
	}
	token := selectNFTAsset(req.Asset)
	if token == nil {
		customInput(&c.Controller, ErrCodeRequest, "NFT Asset not exist")
		return
	}

	item, err := getSingleItem(sdk, wrapper, token, req.TokenId, req.Address)
	if err != nil {
		logs.Error("get single item err: %v", err)
	}

	var (
		items    = make([]*Item, 0)
		totalCnt = 0
	)
	if item != nil {
		items = append(items, item)
		totalCnt = 1
	}
	data := new(ItemsOfAddressRsp).instance(req.PageSize, req.PageNo, 0, totalCnt, items)
	output(&c.Controller, data)
}

func (c *ItemController) batchFetchNFTItems(req *ItemsOfAddressReq) {

	// check params
	sdk := selectNode(req.ChainId)
	if sdk == nil {
		customInput(&c.Controller, ErrCodeRequest, "chain id not exist")
		return
	}
	wrapper := selectWrapper(req.ChainId)
	if wrapper == emptyAddr {
		customInput(&c.Controller, ErrCodeRequest, "chain id not exist")
		return
	}
	token := selectNFTAsset(req.Asset)
	if token == nil {
		customInput(&c.Controller, ErrCodeRequest, "NFT Asset not exist")
		return
	}

	// get user balance and format page attribute
	asset := common.HexToAddress(token.Hash)
	owner := common.HexToAddress(req.Address)
	bigTotalCnt, err := sdk.NFTBalance(asset, owner)
	if err != nil {
		customInput(&c.Controller, ErrCodeRequest, err.Error())
		return
	}
	totalCnt := int(bigTotalCnt.Uint64())
	totalPage := getPageNo(totalCnt, req.PageSize)

	// define empty output
	response := func(list []*Item) {
		data := new(ItemsOfAddressRsp).instance(req.PageSize, req.PageNo, totalPage, totalCnt, list)
		output(&c.Controller, data)
	}

	// check user balance and query index
	if totalCnt == 0 {
		response(nil)
		return
	}
	start := req.PageNo * req.PageSize
	if start >= totalCnt {
		customInput(&c.Controller, ErrCodeRequest, "start out of range")
		return
	}
	length := req.PageSize
	if start+length > totalCnt {
		length = totalCnt - start
	}

	// get token id list from contract, order by index
	tokenIdUrlMap, err := sdk.GetTokensByIndex(wrapper, asset, owner, start, length)
	if err != nil {
		logs.Error("GetTokensByIndex err: %v", err)
		response(nil)
		return
	}
	if len(tokenIdUrlMap) == 0 {
		response(nil)
		return
	}

	items := getItemsWithChainData(token.TokenBasicName, token.Hash, token.ChainId, tokenIdUrlMap)
	response(items)
}

func getSingleItem(sdk *chainsdk.EthereumSdkPro, wrapper common.Address, asset *models.Token, tokenIdStr string, ownerHash string) (*Item, error) {
	tokenId, ok := string2Big(tokenIdStr)
	if !ok {
		return nil, fmt.Errorf("invalid token id")
	}

	// get and output cache if exist
	cache, ok := GetItemCache(asset.ChainId, asset.Hash, tokenIdStr)
	if ok {
		return cache, nil
	}

	// fetch url from wrapper contract
	// do not need to check user address if ownerHash is empty
	var url string
	assetAddr := common.HexToAddress(asset.Hash)
	if ownerHash == "" {
		urlList, err := sdk.GetTokensById(wrapper, assetAddr, []*big.Int{tokenId})
		if err != nil {
			return nil, err
		}
		if len(urlList) == 0 {
			return nil, fmt.Errorf("can not find profile")
		}
		url = urlList[tokenIdStr]
	} else {
		owner := common.HexToAddress(ownerHash)
		checkedUrl, err := sdk.GetAndCheckTokenUrl(wrapper, assetAddr, owner, tokenId)
		if err != nil {
			return nil, err
		}
		url = checkedUrl
	}

	profile, _ := fetcher.Fetch(asset.TokenBasicName, &mcm.FetchRequestParams{
		TokenId: tokenIdStr,
		Url:     url,
	})
	item := new(Item).instance(asset.TokenBasicName, tokenIdStr, profile)
	SetItemCache(asset.ChainId, asset.Hash, tokenIdStr, item)
	return item, nil
}

func getItemsWithChainData(name string, asset string, chainId uint64, tokenIdUrlMap map[string]string) []*Item {
	list := make([]*Item, 0)

	// get cache if exist
	profileReqs := make([]*mcm.FetchRequestParams, 0)
	for tokenId, url := range tokenIdUrlMap {
		cache, ok := GetItemCache(chainId, asset, tokenId)
		if ok {
			list = append(list, cache)
			delete(tokenIdUrlMap, tokenId)
			continue
		}

		req := &mcm.FetchRequestParams{
			TokenId: tokenId,
			Url:     url,
		}
		profileReqs = append(profileReqs, req)
	}

	// fetch meta data list and show rpc time
	tBeforeBatchFetch := time.Now().UnixNano()
	profiles, err := fetcher.BatchFetch(name, profileReqs)
	if err != nil {
		logs.Error("batch fetch NFT profiles err: %v", err)
	}
	tAfterBatchFetch := time.Now().UnixNano()
	debugBatchFetchTime := (tAfterBatchFetch - tBeforeBatchFetch) / int64(time.Microsecond)
	logs.Info("batchFetchNFTItems - batchFetchTime: %d microsecond， profiles %d", debugBatchFetchTime, len(profiles))

	// convert to items
	for _, v := range profiles {
		tokenId := v.NftTokenId
		item := new(Item).instance(name, tokenId, v)
		list = append(list, item)
		SetItemCache(chainId, asset, tokenId, item)
		delete(tokenIdUrlMap, tokenId)
	}

	for tokenId, _ := range tokenIdUrlMap {
		item := new(Item).instance(name, tokenId, nil)
		list = append(list, item)
	}

	if len(list) < 2 {
		return list
	}

	// sort items with token id
	sort.Slice(list, func(i, j int) bool {
		itemi, _ := string2Big(list[i].TokenId)
		itemj, _ := string2Big(list[j].TokenId)
		return itemi.Cmp(itemj) < 0
	})

	return list
}

func string2Big(str string) (*big.Int, bool) {
	return new(big.Int).SetString(str, 10)
}
