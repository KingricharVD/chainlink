package offchainreporting

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	gethCommon "github.com/ethereum/go-ethereum/common"

	"github.com/smartcontractkit/chainlink/core/services/eth"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/chainlink/offchainreporting/confighelper"
	"github.com/smartcontractkit/chainlink/offchainreporting/gethwrappers/offchainaggregator"
	ocrtypes "github.com/smartcontractkit/chainlink/offchainreporting/types"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/pkg/errors"
)

var (
	OCRContractConfigSet = getConfigSetHash()
)

type (
	LogConfigSet struct {
		eth.GethRawLog
		ConfigCount  uint64
		Oracles      []gethCommon.Address
		Transmitters []gethCommon.Address
		Threshold    uint8
		Config       []byte
	}

	OCRContract struct {
		ethClient        eth.Client
		configSetChan    chan offchainaggregator.OffchainAggregatorConfigSet
		contractFilterer *offchainaggregator.OffchainAggregatorFilterer
		contractCaller   *offchainaggregator.OffchainAggregatorCaller
		contractAddress  gethCommon.Address
		logBroadcaster   eth.LogBroadcaster

		jobID       models.ID
		transmitter Transmitter
		contractABI abi.ABI
	}

	Transmitter interface {
		CreateEthTransaction(ctx context.Context, toAddress gethCommon.Address, payload []byte) error
		FromAddress() gethCommon.Address
	}
)

var (
	_ ocrtypes.ContractConfigTracker      = &OCRContract{}
	_ ocrtypes.ContractTransmitter        = &OCRContract{}
	_ ocrtypes.ContractConfigSubscription = &OCRContractConfigSubscription{}
)

func NewOCRContract(address gethCommon.Address, ethClient eth.Client, logBroadcaster eth.LogBroadcaster, jobID models.ID, transmitter Transmitter) (o *OCRContract, err error) {
	contractFilterer, err := offchainaggregator.NewOffchainAggregatorFilterer(address, ethClient)
	if err != nil {
		return o, errors.Wrap(err, "could not instantiate NewOffchainAggregatorFilterer")
	}

	contractCaller, err := offchainaggregator.NewOffchainAggregatorCaller(address, ethClient)
	if err != nil {
		return o, errors.Wrap(err, "could not instantiate NewOffchainAggregatorCaller")
	}

	contractABI, err := abi.JSON(strings.NewReader(offchainaggregator.OffchainAggregatorABI))
	if err != nil {
		return o, errors.Wrap(err, "could not get contract ABI JSON")
	}

	return &OCRContract{
		contractFilterer: contractFilterer,
		contractCaller:   contractCaller,
		contractAddress:  address,
		logBroadcaster:   logBroadcaster,
		ethClient:        ethClient,
		configSetChan:    make(chan offchainaggregator.OffchainAggregatorConfigSet),
		jobID:            jobID,
		transmitter:      transmitter,
		contractABI:      contractABI,
	}, nil
}

func (ra *OCRContract) Transmit(ctx context.Context, report []byte, rs, ss [][32]byte, vs [32]byte) error {
	payload, err := ra.contractABI.Pack("transmit", report, rs, ss, vs)
	if err != nil {
		return errors.Wrap(err, "abi.Pack failed")
	}

	return errors.Wrap(ra.transmitter.CreateEthTransaction(ctx, ra.contractAddress, payload), "failed to send Eth transaction")
}

func (ra *OCRContract) SubscribeToNewConfigs(context.Context) (ocrtypes.ContractConfigSubscription, error) {
	sub := &OCRContractConfigSubscription{
		make(chan ocrtypes.ContractConfig),
		ra,
		sync.Mutex{},
		false,
	}
	connected := ra.logBroadcaster.Register(ra.contractAddress, sub)
	if !connected {
		return nil, errors.New("Failed to register with logBroadcaster")
	}

	return sub, nil
}

func (ra *OCRContract) LatestConfigDetails(ctx context.Context) (changedInBlock uint64, configDigest ocrtypes.ConfigDigest, err error) {
	opts := bind.CallOpts{Context: ctx, Pending: false}
	result, err := ra.contractCaller.LatestConfigDetails(&opts)
	if err != nil {
		return 0, configDigest, errors.Wrap(err, "error getting LatestConfigDetails")
	}
	return uint64(result.BlockNumber), ocrtypes.BytesToConfigDigest(result.ConfigDigest[:]), err
}

// Conform OCRContract to LogListener interface
type OCRContractConfigSubscription struct {
	ch       chan ocrtypes.ContractConfig
	ra       *OCRContract
	mutex    sync.Mutex
	chClosed bool
}

func (sub *OCRContractConfigSubscription) OnConnect() {}
func (sub *OCRContractConfigSubscription) OnDisconnect() {
	sub.mutex.Lock()
	defer sub.mutex.Unlock()

	if !sub.chClosed {
		sub.chClosed = true
		close(sub.ch)
	}
}
func (sub *OCRContractConfigSubscription) HandleLog(lb eth.LogBroadcast, err error) {
	topics := lb.Log().RawLog().Topics
	if len(topics) == 0 {
		return
	}
	switch topics[0] {
	case OCRContractConfigSet:
		configSet, err := sub.ra.contractFilterer.ParseConfigSet(lb.Log().RawLog())
		if err != nil {
			panic(err)
		}
		configSet.Raw = lb.Log().RawLog()
		cc := confighelper.ContractConfigFromConfigSetEvent(*configSet)
		sub.ch <- cc
	default:
	}
}
func (sub *OCRContractConfigSubscription) JobID() *models.ID {
	jobID := sub.ra.jobID
	return &jobID
}
func (sub *OCRContractConfigSubscription) Configs() <-chan ocrtypes.ContractConfig {
	return sub.ch
}
func (sub *OCRContractConfigSubscription) Close() {
	sub.ra.logBroadcaster.Unregister(sub.ra.contractAddress, sub)
}

func (ra *OCRContract) ConfigFromLogs(ctx context.Context, changedInBlock uint64) (c ocrtypes.ContractConfig, err error) {
	q := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(changedInBlock)),
		ToBlock:   big.NewInt(int64(changedInBlock)),
		Addresses: []gethCommon.Address{ra.contractAddress},
		Topics: [][]gethCommon.Hash{
			{OCRContractConfigSet},
		},
	}

	logs, err := ra.ethClient.FilterLogs(ctx, q)
	if err != nil {
		return c, err
	}
	if len(logs) == 0 {
		return c, errors.Errorf("ConfigFromLogs: OCRContract with address 0x%x has no logs", ra.contractAddress)
	}

	latest, err := ra.contractFilterer.ParseConfigSet(logs[len(logs)-1])
	if err != nil {
		return c, errors.Wrap(err, "ConfigFromLogs failed to ParseConfigSet")
	}
	latest.Raw = logs[len(logs)-1]
	return confighelper.ContractConfigFromConfigSetEvent(*latest), err
}

func (ra *OCRContract) LatestBlockHeight(ctx context.Context) (blockheight uint64, err error) {
	h, err := ra.ethClient.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, err
	}
	if h == nil {
		return 0, errors.New("got nil head")
	}

	return uint64(h.Number), nil
}

func (ra *OCRContract) LatestTransmissionDetails(ctx context.Context) (configDigest ocrtypes.ConfigDigest, epoch uint32, round uint8, latestAnswer ocrtypes.Observation, latestTimestamp time.Time, err error) {
	opts := bind.CallOpts{Context: ctx, Pending: false}
	result, err := ra.contractCaller.LatestTransmissionDetails(&opts)
	if err != nil {
		return configDigest, 0, 0, ocrtypes.Observation(nil), time.Time{}, errors.Wrap(err, "error getting LatestTransmissionDetails")
	}
	return result.ConfigDigest, result.Epoch, result.Round, ocrtypes.Observation(result.LatestAnswer), time.Unix(int64(result.LatestTimestamp), 0), nil
}

func getConfigSetHash() gethCommon.Hash {
	abi, err := abi.JSON(strings.NewReader(offchainaggregator.OffchainAggregatorABI))
	if err != nil {
		panic("could not parse OffchainAggregator ABI: " + err.Error())
	}
	return abi.Events["ConfigSet"].ID
}

func (ra *OCRContract) FromAddress() gethCommon.Address {
	return ra.transmitter.FromAddress()
}
