package bbgo

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

type PnLReporterConfig struct {
	AverageCostBySymbols StringSlice `json:"averageCostBySymbols" yaml:"averageCostBySymbols"`
	Of                   StringSlice `json:"of" yaml:"of"`
	When                 StringSlice `json:"when" yaml:"when"`
}

// ExchangeStrategyMount wraps the SingleExchangeStrategy with the session name for mounting
type ExchangeStrategyMount struct {
	// Mounts contains the session name to mount
	Mounts []string

	// Strategy is the strategy we loaded from config
	Strategy SingleExchangeStrategy
}

type SlackNotification struct {
	DefaultChannel string `json:"defaultChannel,omitempty"  yaml:"defaultChannel,omitempty"`
	ErrorChannel   string `json:"errorChannel,omitempty"  yaml:"errorChannel,omitempty"`
}

type NotificationRouting struct {
	Trade       string `json:"trade,omitempty" yaml:"trade,omitempty"`
	Order       string `json:"order,omitempty" yaml:"order,omitempty"`
	SubmitOrder string `json:"submitOrder,omitempty" yaml:"submitOrder,omitempty"`
	PnL         string `json:"pnL,omitempty" yaml:"pnL,omitempty"`
}

type NotificationConfig struct {
	Slack *SlackNotification `json:"slack,omitempty" yaml:"slack,omitempty"`

	SymbolChannels  map[string]string `json:"symbolChannels,omitempty" yaml:"symbolChannels,omitempty"`
	SessionChannels map[string]string `json:"sessionChannels,omitempty" yaml:"sessionChannels,omitempty"`

	Routing *NotificationRouting `json:"routing,omitempty" yaml:"routing,omitempty"`
}

type Session struct {
	ExchangeName string `json:"exchange" yaml:"exchange"`
	EnvVarPrefix string `json:"envVarPrefix" yaml:"envVarPrefix"`
}

type Backtest struct {
	StartTime string `json:"startTime" yaml:"startTime"`
	EndTime   string `json:"endTime" yaml:"endTime"`

	Account BacktestAccount `json:"account" yaml:"account"`
	Symbols []string        `json:"symbols" yaml:"symbols"`
}

func (t Backtest) ParseEndTime() (time.Time, error) {
	if len(t.EndTime) == 0 {
		return time.Time{}, errors.New("backtest.endTime must be defined")
	}

	return time.Parse("2006-01-02", t.EndTime)
}

func (t Backtest) ParseStartTime() (time.Time, error) {
	if len(t.StartTime) == 0 {
		return time.Time{}, errors.New("backtest.startTime must be defined")
	}

	return time.Parse("2006-01-02", t.StartTime)
}

type BacktestAccount struct {
	MakerCommission  int                       `json:"makerCommission"`
	TakerCommission  int                       `json:"takerCommission"`
	BuyerCommission  int                       `json:"buyerCommission"`
	SellerCommission int                       `json:"sellerCommission"`
	Balances         BacktestAccountBalanceMap `json:"balances" yaml:"balances"`
}

type BacktestAccountBalanceMap map[string]fixedpoint.Value

func (m BacktestAccountBalanceMap) BalanceMap() types.BalanceMap {
	balances := make(types.BalanceMap)
	for currency, value := range m {
		balances[currency] = types.Balance{
			Currency:  currency,
			Available: value,
			Locked:    0,
		}
	}
	return balances
}

type Config struct {
	Imports []string `json:"imports" yaml:"imports"`

	Backtest *Backtest `json:"backtest,omitempty" yaml:"backtest,omitempty"`

	Notifications *NotificationConfig `json:"notifications,omitempty" yaml:"notifications,omitempty"`

	Sessions map[string]Session `json:"sessions,omitempty" yaml:"sessions,omitempty"`

	RiskControls *RiskControls `json:"riskControls,omitempty" yaml:"riskControls,omitempty"`

	ExchangeStrategies      []ExchangeStrategyMount
	CrossExchangeStrategies []CrossExchangeStrategy

	PnLReporters []PnLReporterConfig `json:"reportPnL,omitempty" yaml:"reportPnL,omitempty"`
}

type Stash map[string]interface{}

func loadStash(config []byte) (Stash, error) {
	stash := make(Stash)
	if err := yaml.Unmarshal(config, stash); err != nil {
		return nil, err
	}

	return stash, nil
}

func Preload(configFile string) (*Config, error) {
	var config Config

	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func Load(configFile string) (*Config, error) {
	var config Config

	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	stash, err := loadStash(content)
	if err != nil {
		return nil, err
	}

	if err := loadExchangeStrategies(&config, stash); err != nil {
		return nil, err
	}

	if err := loadCrossExchangeStrategies(&config, stash); err != nil {
		return nil, err
	}

	return &config, nil
}

func loadCrossExchangeStrategies(config *Config, stash Stash) (err error) {
	exchangeStrategiesConf, ok := stash["crossExchangeStrategies"]
	if !ok {
		return nil
	}

	if len(LoadedCrossExchangeStrategies) == 0 {
		return errors.New("no cross exchange strategy is registered")
	}

	configList, ok := exchangeStrategiesConf.([]interface{})
	if !ok {
		return errors.New("expecting list in crossExchangeStrategies")
	}

	for _, entry := range configList {
		configStash, ok := entry.(Stash)
		if !ok {
			return fmt.Errorf("strategy config should be a map, given: %T %+v", entry, entry)
		}

		for id, conf := range configStash {
			// look up the real struct type
			if st, ok := LoadedExchangeStrategies[id]; ok {
				val, err := reUnmarshal(conf, st)
				if err != nil {
					return err
				}

				config.CrossExchangeStrategies = append(config.CrossExchangeStrategies, val.(CrossExchangeStrategy))
			}
		}
	}

	return nil
}

func loadExchangeStrategies(config *Config, stash Stash) (err error) {
	exchangeStrategiesConf, ok := stash["exchangeStrategies"]
	if !ok {
		return nil
	}

	if len(LoadedExchangeStrategies) == 0 {
		return errors.New("no exchange strategy is registered")
	}

	configList, ok := exchangeStrategiesConf.([]interface{})
	if !ok {
		return errors.New("expecting list in exchangeStrategies")
	}

	for _, entry := range configList {
		configStash, ok := entry.(Stash)
		if !ok {
			return fmt.Errorf("strategy config should be a map, given: %T %+v", entry, entry)
		}

		var mounts []string
		if val, ok := configStash["on"]; ok {
			if values, ok := val.([]string); ok {
				mounts = append(mounts, values...)
			} else if str, ok := val.(string); ok {
				mounts = append(mounts, str)
			}
		}

		for id, conf := range configStash {
			// look up the real struct type
			if st, ok := LoadedExchangeStrategies[id]; ok {
				val, err := reUnmarshal(conf, st)
				if err != nil {
					return err
				}

				config.ExchangeStrategies = append(config.ExchangeStrategies, ExchangeStrategyMount{
					Mounts:   mounts,
					Strategy: val.(SingleExchangeStrategy),
				})
			}
		}
	}

	return nil
}

func reUnmarshal(conf interface{}, tpe interface{}) (interface{}, error) {
	// get the type "*Strategy"
	rt := reflect.TypeOf(tpe)

	// allocate new object from the given type
	val := reflect.New(rt)

	// now we have &(*Strategy) -> **Strategy
	valRef := val.Interface()

	plain, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(plain, valRef); err != nil {
		return nil, errors.Wrapf(err, "json parsing error, given payload: %s", plain)
	}

	return val.Elem().Interface(), nil
}
