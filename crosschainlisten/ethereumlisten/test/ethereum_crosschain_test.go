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

package test

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"poly-bridge/basedef"
	"poly-bridge/chainsdk"
	"poly-bridge/conf"
	"poly-bridge/go_abi/wrapper_abi"
	"strings"
	"testing"
)

func NewPrivateKey(key string) *ecdsa.PrivateKey {
	priKey, err := crypto.HexToECDSA(key)
	if err != nil {
		panic(err)
	}
	return priKey
}

func TestEthereumCross(t *testing.T) {
	config := conf.NewConfig("./../../../conf/config_testnet.json")
	if config == nil {
		panic("read config failed!")
	}
	ethChainListenConfig := config.GetChainListenConfig(basedef.ETHEREUM_CROSSCHAIN_ID)
	urls := ethChainListenConfig.GetNodesUrl()
	ethSdk := chainsdk.NewEthereumSdkPro(urls, ethChainListenConfig.ListenSlot, basedef.ETHEREUM_CROSSCHAIN_ID)
	contractabi, err := abi.JSON(strings.NewReader(wrapper_abi.IPolyWrapperABI))
	if err != nil {
		panic(err)
	}
	assetHash := common.HexToAddress("0000000000000000000000000000000000000000")
	toAddress := common.Hex2Bytes("6e43f9988f2771f1a2b140cb3faad424767d39fc")
	txData, err := contractabi.Pack("lock", assetHash, uint64(79), toAddress, big.NewInt(int64(100000000000000000)), big.NewInt(10000000000000000))
	if err != nil {
		panic(err)
	}
	fmt.Printf("TestInvokeContract - txdata:%s\n", hex.EncodeToString(txData))
	wrapperContractAddress := common.HexToAddress(ethChainListenConfig.WrapperContract)
	privateKey := NewPrivateKey("56b446a2de5edfccee1581fbba79e8bb5c269e28ab4c0487860afb7e2c2d2b6e")
	fromAddr := crypto.PubkeyToAddress(privateKey.PublicKey)
	fmt.Printf("user address: %s\n", fromAddr.String())
	nonce, err := ethSdk.NonceAt(fromAddr)
	if err != nil {
		panic(err)
	}
	gasPrice, err := ethSdk.SuggestGasPrice()
	if err != nil {
		panic(err)
	}
	fmt.Printf("gas price: %s\n", gasPrice.String())
	callMsg := ethereum.CallMsg{
		From: fromAddr, To: &wrapperContractAddress, Gas: 0, GasPrice: gasPrice,
		Value: big.NewInt(100000000000000000), Data: txData,
	}

	gasLimit, err := ethSdk.EstimateGas(callMsg)
	if err != nil || gasLimit == 0 {
		panic(err)
	}
	fmt.Printf("gas limit: %d\n", gasLimit)
	tx := types.NewTransaction(nonce, wrapperContractAddress, big.NewInt(100000000000000000), gasLimit, gasPrice, txData)
	signedTx, err := types.SignTx(tx, types.HomesteadSigner{}, privateKey)
	if err != nil {
		panic(err)
	}
	err = ethSdk.SendRawTransaction(signedTx)
	if err != nil {
		panic(err)
	}
	ethSdk.WaitTransactionConfirm(signedTx.Hash())
}

func TestEthereum_GetFeeCollector(t *testing.T) {
	config := conf.NewConfig("./../../../conf/config_testnet.json")
	if config == nil {
		panic("read config failed!")
	}
	ethChainListenConfig := config.GetChainListenConfig(basedef.ETHEREUM_CROSSCHAIN_ID)
	address := common.HexToAddress(ethChainListenConfig.WrapperContract)
	urls := ethChainListenConfig.GetNodesUrl()
	ethSdk := chainsdk.NewEthereumSdkPro(urls, ethChainListenConfig.ListenSlot, basedef.ETHEREUM_CROSSCHAIN_ID)
	instance, err := wrapper_abi.NewIPolyWrapper(address, ethSdk.GetClient())
	if err != nil {
		panic(err)
	}
	collector, _ := instance.FeeCollector(nil)
	fmt.Printf("collector: %s\n", collector.String())
	lockproxy, _ := instance.LockProxy(nil)
	fmt.Printf("lock proxy: %s\n", lockproxy.String())
	owner, _ := instance.Owner(nil)
	fmt.Printf("owner: %s\n", owner.String())
}

func TestEthereumExtractFee(t *testing.T) {
	config := conf.NewConfig("./../../../conf/config_testnet.json")
	if config == nil {
		panic("read config failed!")
	}
	ethChainListenConfig := config.GetChainListenConfig(basedef.ETHEREUM_CROSSCHAIN_ID)
	urls := ethChainListenConfig.GetNodesUrl()
	ethSdk := chainsdk.NewEthereumSdkPro(urls, ethChainListenConfig.ListenSlot, basedef.ETHEREUM_CROSSCHAIN_ID)
	contractabi, err := abi.JSON(strings.NewReader(wrapper_abi.IPolyWrapperABI))
	if err != nil {
		panic(err)
	}

	assetHash := common.HexToAddress("0000000000000000000000000000000000000000")
	txData, err := contractabi.Pack("extractFee", assetHash)
	if err != nil {
		panic(err)
	}
	fmt.Printf("TestInvokeContract - txdata:%s\n", hex.EncodeToString(txData))
	wrapperContractAddress := common.HexToAddress(ethChainListenConfig.WrapperContract)
	privateKey := NewPrivateKey("56b446a2de5edfccee1581fbba79e8bb5c269e28ab4c0487860afb7e2c2d2b6e")
	fromAddr := crypto.PubkeyToAddress(privateKey.PublicKey)
	fmt.Printf("user address: %s\n", fromAddr.String())
	nonce, err := ethSdk.NonceAt(fromAddr)
	if err != nil {
		panic(err)
	}
	gasPrice, err := ethSdk.SuggestGasPrice()
	if err != nil {
		panic(err)
	}
	fmt.Printf("gas price: %s\n", gasPrice.String())
	callMsg := ethereum.CallMsg{
		From: fromAddr, To: &wrapperContractAddress, Gas: 0, GasPrice: gasPrice,
		Value: big.NewInt(0), Data: txData,
	}

	gasLimit, err := ethSdk.EstimateGas(callMsg)
	if err != nil || gasLimit == 0 {
		panic(err)
	}
	fmt.Printf("gas limit: %d\n", gasLimit)
	tx := types.NewTransaction(nonce, wrapperContractAddress, big.NewInt(0), gasLimit, gasPrice, txData)
	signedTx, err := types.SignTx(tx, types.HomesteadSigner{}, privateKey)
	if err != nil {
		panic(err)
	}
	err = ethSdk.SendRawTransaction(signedTx)
	if err != nil {
		panic(err)
	}
	ethSdk.WaitTransactionConfirm(signedTx.Hash())
}

func TestEthereumSpeedup(t *testing.T) {
	config := conf.NewConfig("./../../../conf/config_testnet.json")
	if config == nil {
		panic("read config failed!")
	}
	ethChainListenConfig := config.GetChainListenConfig(basedef.ETHEREUM_CROSSCHAIN_ID)
	urls := ethChainListenConfig.GetNodesUrl()
	ethSdk := chainsdk.NewEthereumSdkPro(urls, ethChainListenConfig.ListenSlot, basedef.ETHEREUM_CROSSCHAIN_ID)
	contractabi, err := abi.JSON(strings.NewReader(wrapper_abi.IPolyWrapperABI))
	if err != nil {
		panic(err)
	}

	assetHash := common.HexToAddress("0000000000000000000000000000000000000000")
	txData, err := contractabi.Pack("extractFee", assetHash)
	if err != nil {
		panic(err)
	}
	fmt.Printf("TestInvokeContract - txdata:%s\n", hex.EncodeToString(txData))
	wrapperContractAddress := common.HexToAddress(ethChainListenConfig.WrapperContract)
	privateKey := NewPrivateKey("56b446a2de5edfccee1581fbba79e8bb5c269e28ab4c0487860afb7e2c2d2b6e")
	fromAddr := crypto.PubkeyToAddress(privateKey.PublicKey)
	fmt.Printf("user address: %s\n", fromAddr.String())
	nonce, err := ethSdk.NonceAt(fromAddr)
	if err != nil {
		panic(err)
	}
	gasPrice, err := ethSdk.SuggestGasPrice()
	if err != nil {
		panic(err)
	}
	fmt.Printf("gas price: %s\n", gasPrice.String())
	callMsg := ethereum.CallMsg{
		From: fromAddr, To: &wrapperContractAddress, Gas: 0, GasPrice: gasPrice,
		Value: big.NewInt(0), Data: txData,
	}

	gasLimit, err := ethSdk.EstimateGas(callMsg)
	if err != nil || gasLimit == 0 {
		panic(err)
	}
	fmt.Printf("gas limit: %d\n", gasLimit)
	tx := types.NewTransaction(nonce, wrapperContractAddress, big.NewInt(0), gasLimit, gasPrice, txData)
	signedTx, err := types.SignTx(tx, types.HomesteadSigner{}, privateKey)
	if err != nil {
		panic(err)
	}
	err = ethSdk.SendRawTransaction(signedTx)
	if err != nil {
		panic(err)
	}
	ethSdk.WaitTransactionConfirm(signedTx.Hash())
}
