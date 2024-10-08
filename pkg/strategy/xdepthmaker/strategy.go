package xdepthmaker

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/core"
	"github.com/c9s/bbgo/pkg/exchange/retry"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/strategy/common"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/c9s/bbgo/pkg/util"
)

var lastPriceModifier = fixedpoint.NewFromFloat(1.001)
var minGap = fixedpoint.NewFromFloat(1.02)
var defaultMargin = fixedpoint.NewFromFloat(0.003)

var Two = fixedpoint.NewFromInt(2)

const priceUpdateTimeout = 5 * time.Minute

const ID = "xdepthmaker"

var log = logrus.WithField("strategy", ID)

func init() {
	bbgo.RegisterStrategy(ID, &Strategy{})
}

type CrossExchangeMarketMakingStrategy struct {
	ctx, parent context.Context
	cancel      context.CancelFunc

	Environ *bbgo.Environment

	makerSession, hedgeSession *bbgo.ExchangeSession
	makerMarket, hedgeMarket   types.Market

	// persistence fields
	Position        *types.Position    `json:"position,omitempty" persistence:"position"`
	ProfitStats     *types.ProfitStats `json:"profitStats,omitempty" persistence:"profit_stats"`
	CoveredPosition fixedpoint.Value   `json:"coveredPosition,omitempty" persistence:"covered_position"`

	core.ConverterManager

	mu sync.Mutex

	MakerOrderExecutor, HedgeOrderExecutor *bbgo.GeneralOrderExecutor
}

func (s *CrossExchangeMarketMakingStrategy) Initialize(
	ctx context.Context, environ *bbgo.Environment,
	makerSession, hedgeSession *bbgo.ExchangeSession,
	symbol, hedgeSymbol,
	strategyID, instanceID string,
) error {
	s.parent = ctx
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.Environ = environ

	s.makerSession = makerSession
	s.hedgeSession = hedgeSession

	var ok bool
	s.hedgeMarket, ok = s.hedgeSession.Market(hedgeSymbol)
	if !ok {
		return fmt.Errorf("hedge session market %s is not defined", hedgeSymbol)
	}

	s.makerMarket, ok = s.makerSession.Market(symbol)
	if !ok {
		return fmt.Errorf("maker session market %s is not defined", symbol)
	}

	if err := s.ConverterManager.Initialize(); err != nil {
		return err
	}

	if s.ProfitStats == nil {
		s.ProfitStats = types.NewProfitStats(s.makerMarket)
	}

	if s.Position == nil {
		s.Position = types.NewPositionFromMarket(s.makerMarket)
	}

	// Always update the position fields
	s.Position.Strategy = strategyID
	s.Position.StrategyInstanceID = instanceID

	// if anyone of the fee rate is defined, this assumes that both are defined.
	// so that zero maker fee could be applied
	for _, ses := range []*bbgo.ExchangeSession{makerSession, hedgeSession} {
		if ses.MakerFeeRate.Sign() > 0 || ses.TakerFeeRate.Sign() > 0 {
			s.Position.SetExchangeFeeRate(ses.ExchangeName, types.ExchangeFee{
				MakerFeeRate: ses.MakerFeeRate,
				TakerFeeRate: ses.TakerFeeRate,
			})
		}
	}

	s.MakerOrderExecutor = bbgo.NewGeneralOrderExecutor(
		makerSession,
		s.makerMarket.Symbol,
		strategyID, instanceID,
		s.Position)

	// update converter manager
	s.MakerOrderExecutor.TradeCollector().ConverterManager = s.ConverterManager

	s.MakerOrderExecutor.BindEnvironment(environ)
	s.MakerOrderExecutor.BindProfitStats(s.ProfitStats)
	s.MakerOrderExecutor.Bind()
	s.MakerOrderExecutor.TradeCollector().OnPositionUpdate(func(position *types.Position) {
		// bbgo.Sync(ctx, s)
	})

	s.HedgeOrderExecutor = bbgo.NewGeneralOrderExecutor(
		hedgeSession,
		s.hedgeMarket.Symbol,
		strategyID, instanceID,
		s.Position)
	s.HedgeOrderExecutor.BindEnvironment(environ)
	s.HedgeOrderExecutor.BindProfitStats(s.ProfitStats)
	s.HedgeOrderExecutor.Bind()

	s.HedgeOrderExecutor.TradeCollector().ConverterManager = s.ConverterManager

	s.HedgeOrderExecutor.TradeCollector().OnPositionUpdate(func(position *types.Position) {
		// bbgo.Sync(ctx, s)
	})

	s.HedgeOrderExecutor.TradeCollector().OnTrade(func(trade types.Trade, profit, netProfit fixedpoint.Value) {
		c := trade.PositionChange()

		// sync covered position
		// sell trade -> negative delta ->
		// 	  1) long position -> reduce long position
		//    2) short position -> increase short position
		// buy trade -> positive delta ->
		// 	  1) short position -> reduce short position
		// 	  2) short position -> increase short position

		// TODO: make this atomic
		s.mu.Lock()
		s.CoveredPosition = s.CoveredPosition.Add(c)
		s.mu.Unlock()
	})
	return nil
}

type Strategy struct {
	*CrossExchangeMarketMakingStrategy

	Environment *bbgo.Environment

	// Symbol is the maker exchange symbol
	Symbol string `json:"symbol"`

	// HedgeSymbol is the symbol for the hedge exchange
	// symbol could be different from the maker exchange
	HedgeSymbol string `json:"hedgeSymbol"`

	// MakerExchange session name
	MakerExchange string `json:"makerExchange"`

	// HedgeExchange session name
	HedgeExchange string `json:"hedgeExchange"`

	UpdateInterval types.Duration `json:"updateInterval"`

	HedgeInterval types.Duration `json:"hedgeInterval"`

	FullReplenishInterval types.Duration `json:"fullReplenishInterval"`

	OrderCancelWaitTime types.Duration `json:"orderCancelWaitTime"`

	Margin    fixedpoint.Value `json:"margin"`
	BidMargin fixedpoint.Value `json:"bidMargin"`
	AskMargin fixedpoint.Value `json:"askMargin"`

	StopHedgeQuoteBalance fixedpoint.Value `json:"stopHedgeQuoteBalance"`
	StopHedgeBaseBalance  fixedpoint.Value `json:"stopHedgeBaseBalance"`

	// Quantity is used for fixed quantity of the first layer
	Quantity fixedpoint.Value `json:"quantity"`

	// QuantityScale helps user to define the quantity by layer scale
	QuantityScale *bbgo.LayerScale `json:"quantityScale,omitempty"`

	// DepthScale helps user to define the depth by layer scale
	DepthScale *bbgo.LayerScale `json:"depthScale,omitempty"`

	// MaxExposurePosition defines the unhedged quantity of stop
	MaxExposurePosition fixedpoint.Value `json:"maxExposurePosition"`

	DisableHedge bool `json:"disableHedge"`

	NotifyTrade bool `json:"notifyTrade"`

	// RecoverTrade tries to find the missing trades via the REStful API
	RecoverTrade bool `json:"recoverTrade"`

	RecoverTradeScanPeriod types.Duration `json:"recoverTradeScanPeriod"`

	NumLayers int `json:"numLayers"`

	// Pips is the pips of the layer prices
	Pips fixedpoint.Value `json:"pips"`

	ProfitFixerConfig *common.ProfitFixerConfig `json:"profitFixer,omitempty"`

	// --------------------------------
	// private fields
	// --------------------------------

	// pricingBook is the order book (depth) from the hedging session
	pricingBook *types.StreamOrderBook

	hedgeErrorLimiter         *rate.Limiter
	hedgeErrorRateReservation *rate.Reservation

	askPriceHeartBeat, bidPriceHeartBeat *types.PriceHeartBeat

	lastPrice fixedpoint.Value

	stopC, authedC chan struct{}
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) InstanceID() string {
	return fmt.Sprintf("%s:%s:%s-%s", ID, s.Symbol, s.MakerExchange, s.HedgeExchange)
}

func (s *Strategy) Initialize() error {
	if s.CrossExchangeMarketMakingStrategy == nil {
		s.CrossExchangeMarketMakingStrategy = &CrossExchangeMarketMakingStrategy{}
	}

	s.bidPriceHeartBeat = types.NewPriceHeartBeat(priceUpdateTimeout)
	s.askPriceHeartBeat = types.NewPriceHeartBeat(priceUpdateTimeout)
	return nil
}

func (s *Strategy) CrossSubscribe(sessions map[string]*bbgo.ExchangeSession) {
	makerSession, hedgeSession, err := selectSessions2(sessions, s.MakerExchange, s.HedgeExchange)
	if err != nil {
		panic(err)
	}

	hedgeSession.Subscribe(types.BookChannel, s.HedgeSymbol, types.SubscribeOptions{
		Depth: types.DepthLevelMedium,
		Speed: types.SpeedLow,
	})

	hedgeSession.Subscribe(types.KLineChannel, s.HedgeSymbol, types.SubscribeOptions{Interval: "1m"})

	makerSession.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: "1m"})
}

func (s *Strategy) Validate() error {
	if s.MakerExchange == "" {
		return errors.New("maker exchange is not configured")
	}

	if s.HedgeExchange == "" {
		return errors.New("maker exchange is not configured")
	}

	if s.DepthScale == nil {
		return errors.New("depthScale can not be empty")
	}

	if len(s.Symbol) == 0 {
		return errors.New("symbol is required")
	}

	return nil
}

func (s *Strategy) Defaults() error {
	if s.UpdateInterval == 0 {
		s.UpdateInterval = types.Duration(5 * time.Second)
	}

	if s.FullReplenishInterval == 0 {
		s.FullReplenishInterval = types.Duration(10 * time.Minute)
	}

	if s.HedgeInterval == 0 {
		s.HedgeInterval = types.Duration(3 * time.Second)
	}

	if s.HedgeSymbol == "" {
		s.HedgeSymbol = s.Symbol
	}

	if s.NumLayers == 0 {
		s.NumLayers = 1
	}

	if s.Margin.IsZero() {
		s.Margin = defaultMargin
	}

	if s.BidMargin.IsZero() {
		if !s.Margin.IsZero() {
			s.BidMargin = s.Margin
		} else {
			s.BidMargin = defaultMargin
		}
	}

	if s.AskMargin.IsZero() {
		if !s.Margin.IsZero() {
			s.AskMargin = s.Margin
		} else {
			s.AskMargin = defaultMargin
		}
	}

	s.hedgeErrorLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 1)
	return nil
}

func (s *Strategy) CrossRun(
	ctx context.Context, _ bbgo.OrderExecutionRouter,
	sessions map[string]*bbgo.ExchangeSession,
) error {
	makerSession, hedgeSession, err := selectSessions2(sessions, s.MakerExchange, s.HedgeExchange)
	if err != nil {
		return err
	}

	log.Infof("makerSession: %s hedgeSession: %s", makerSession.Name, hedgeSession.Name)

	if s.ProfitFixerConfig != nil {
		bbgo.Notify("Fixing %s profitStats and position...", s.Symbol)

		log.Infof("profitFixer is enabled, checking checkpoint: %+v", s.ProfitFixerConfig.TradesSince)

		if s.ProfitFixerConfig.TradesSince.Time().IsZero() {
			return errors.New("tradesSince time can not be zero")
		}

		makerMarket, _ := makerSession.Market(s.Symbol)
		s.CrossExchangeMarketMakingStrategy.Position = types.NewPositionFromMarket(makerMarket)
		s.CrossExchangeMarketMakingStrategy.ProfitStats = types.NewProfitStats(makerMarket)

		fixer := common.NewProfitFixer()
		fixer.ConverterManager = s.ConverterManager

		if ss, ok := makerSession.Exchange.(types.ExchangeTradeHistoryService); ok {
			log.Infof("adding makerSession %s to profitFixer", makerSession.Name)
			fixer.AddExchange(makerSession.Name, ss)
		}

		if ss, ok := hedgeSession.Exchange.(types.ExchangeTradeHistoryService); ok {
			log.Infof("adding hedgeSession %s to profitFixer", hedgeSession.Name)
			fixer.AddExchange(hedgeSession.Name, ss)
		}

		if err2 := fixer.Fix(ctx, makerMarket.Symbol,
			s.ProfitFixerConfig.TradesSince.Time(),
			time.Now(),
			s.CrossExchangeMarketMakingStrategy.ProfitStats,
			s.CrossExchangeMarketMakingStrategy.Position); err2 != nil {
			return err2
		}

		bbgo.Notify("Fixed %s position", s.Symbol, s.CrossExchangeMarketMakingStrategy.Position)
		bbgo.Notify("Fixed %s profitStats", s.Symbol, s.CrossExchangeMarketMakingStrategy.ProfitStats)
	}

	if err := s.CrossExchangeMarketMakingStrategy.Initialize(ctx,
		s.Environment,
		makerSession, hedgeSession,
		s.Symbol, s.HedgeSymbol,
		ID, s.InstanceID()); err != nil {
		return err
	}

	s.pricingBook = types.NewStreamBook(s.HedgeSymbol, s.hedgeSession.ExchangeName)
	s.pricingBook.BindStream(s.hedgeSession.MarketDataStream)

	s.stopC = make(chan struct{})

	s.authedC = make(chan struct{}, 5)
	bindAuthSignal(ctx, s.makerSession.UserDataStream, s.authedC)
	bindAuthSignal(ctx, s.hedgeSession.UserDataStream, s.authedC)

	if s.RecoverTrade {
		go s.runTradeRecover(ctx)
	}

	go func() {
		log.Infof("waiting for user data stream to get authenticated")
		select {
		case <-ctx.Done():
			return
		case <-s.authedC:
		}

		select {
		case <-ctx.Done():
			return
		case <-s.authedC:
		}

		log.Infof("user data stream authenticated, start placing orders...")

		posTicker := time.NewTicker(util.MillisecondsJitter(s.HedgeInterval.Duration(), 200))
		defer posTicker.Stop()

		fullReplenishTicker := time.NewTicker(util.MillisecondsJitter(s.FullReplenishInterval.Duration(), 200))
		defer fullReplenishTicker.Stop()

		// clean up the previous open orders
		if err := s.cleanUpOpenOrders(ctx, s.makerSession); err != nil {
			log.WithError(err).Errorf("error cleaning up open orders")
		}

		s.updateQuote(ctx, 0)

		lastOrderReplenishTime := time.Now()
		for {
			select {

			case <-s.stopC:
				log.Warnf("%s maker goroutine stopped, due to the stop signal", s.Symbol)
				return

			case <-ctx.Done():
				log.Warnf("%s maker goroutine stopped, due to the cancelled context", s.Symbol)
				return

			case <-fullReplenishTicker.C:
				s.updateQuote(ctx, 0)
				lastOrderReplenishTime = time.Now()

			case sig, ok := <-s.pricingBook.C:
				// when any book change event happened
				if !ok {
					return
				}

				if time.Since(lastOrderReplenishTime) < 10*time.Second {
					continue
				}

				switch sig.Type {
				case types.BookSignalSnapshot:
					s.updateQuote(ctx, 0)

				case types.BookSignalUpdate:
					s.updateQuote(ctx, 5)
				}

				lastOrderReplenishTime = time.Now()

			case <-posTicker.C:
				// For positive position and positive covered position:
				// uncover position = +5 - +3 (covered position) = 2
				//
				// For positive position and negative covered position:
				// uncover position = +5 - (-3) (covered position) = 8
				//
				// meaning we bought 5 on MAX and sent buy order with 3 on binance
				//
				// For negative position:
				// uncover position = -5 - -3 (covered position) = -2
				s.HedgeOrderExecutor.TradeCollector().Process()
				s.MakerOrderExecutor.TradeCollector().Process()

				position := s.Position.GetBase()
				uncoverPosition := position.Sub(s.CoveredPosition)
				absPos := uncoverPosition.Abs()
				if absPos.Compare(s.hedgeMarket.MinQuantity) > 0 {
					log.Infof("%s base position %v coveredPosition: %v uncoverPosition: %v",
						s.Symbol,
						position,
						s.CoveredPosition,
						uncoverPosition,
					)

					if !s.DisableHedge {
						s.Hedge(ctx, uncoverPosition.Neg())
					}
				}
			}
		}
	}()

	bbgo.OnShutdown(ctx, func(ctx context.Context, wg *sync.WaitGroup) {
		defer wg.Done()

		close(s.stopC)

		// wait for the quoter to stop
		time.Sleep(s.UpdateInterval.Duration())

		if err := s.MakerOrderExecutor.GracefulCancel(ctx); err != nil {
			log.WithError(err).Errorf("graceful cancel %s order error", s.Symbol)
		}

		if err := s.HedgeOrderExecutor.GracefulCancel(ctx); err != nil {
			log.WithError(err).Errorf("graceful cancel %s order error", s.HedgeSymbol)
		}

		bbgo.Sync(ctx, s)
		bbgo.Notify("%s: %s position", ID, s.Symbol, s.Position)
	})

	return nil
}

func (s *Strategy) Hedge(ctx context.Context, pos fixedpoint.Value) {
	side := types.SideTypeBuy
	if pos.IsZero() {
		return
	}

	quantity := pos.Abs()

	if pos.Sign() < 0 {
		side = types.SideTypeSell
	}

	lastPrice := s.lastPrice
	sourceBook := s.pricingBook.CopyDepth(1)
	switch side {

	case types.SideTypeBuy:
		if bestAsk, ok := sourceBook.BestAsk(); ok {
			lastPrice = bestAsk.Price
		}

	case types.SideTypeSell:
		if bestBid, ok := sourceBook.BestBid(); ok {
			lastPrice = bestBid.Price
		}
	}

	notional := quantity.Mul(lastPrice)
	if notional.Compare(s.hedgeMarket.MinNotional) <= 0 {
		log.Warnf("%s %v less than min notional, skipping hedge", s.Symbol, notional)
		return
	}

	// adjust quantity according to the balances
	account := s.hedgeSession.GetAccount()
	switch side {

	case types.SideTypeBuy:
		// check quote quantity
		if quote, ok := account.Balance(s.hedgeMarket.QuoteCurrency); ok {
			if quote.Available.Compare(notional) < 0 {
				// adjust price to higher 0.1%, so that we can ensure that the order can be executed
				quantity = bbgo.AdjustQuantityByMaxAmount(quantity, lastPrice.Mul(lastPriceModifier), quote.Available)
				quantity = s.hedgeMarket.TruncateQuantity(quantity)
			}
		}

	case types.SideTypeSell:
		// check quote quantity
		if base, ok := account.Balance(s.hedgeMarket.BaseCurrency); ok {
			if base.Available.Compare(quantity) < 0 {
				quantity = base.Available
			}
		}
	}

	// truncate quantity for the supported precision
	quantity = s.hedgeMarket.TruncateQuantity(quantity)

	if notional.Compare(s.hedgeMarket.MinNotional.Mul(minGap)) <= 0 {
		log.Warnf("the adjusted amount %v is less than minimal notional %v, skipping hedge", notional, s.hedgeMarket.MinNotional)
		return
	}

	if quantity.Compare(s.hedgeMarket.MinQuantity.Mul(minGap)) <= 0 {
		log.Warnf("the adjusted quantity %v is less than minimal quantity %v, skipping hedge", quantity, s.hedgeMarket.MinQuantity)
		return
	}

	if s.hedgeErrorRateReservation != nil {
		if !s.hedgeErrorRateReservation.OK() {
			return
		}
		bbgo.Notify("Hit hedge error rate limit, waiting...")
		time.Sleep(s.hedgeErrorRateReservation.Delay())
		s.hedgeErrorRateReservation = nil
	}

	log.Infof("submitting %s hedge order %s %v", s.HedgeSymbol, side.String(), quantity)
	bbgo.Notify("Submitting %s hedge order %s %v", s.HedgeSymbol, side.String(), quantity)

	_, err := s.HedgeOrderExecutor.SubmitOrders(ctx, types.SubmitOrder{
		Market:   s.hedgeMarket,
		Symbol:   s.hedgeMarket.Symbol,
		Type:     types.OrderTypeMarket,
		Side:     side,
		Quantity: quantity,
	})

	if err != nil {
		s.hedgeErrorRateReservation = s.hedgeErrorLimiter.Reserve()
		log.WithError(err).Errorf("market order submit error: %s", err.Error())
		return
	}

	// if the hedge is on sell side, then we should add positive position
	switch side {
	case types.SideTypeSell:
		s.mu.Lock()
		s.CoveredPosition = s.CoveredPosition.Add(quantity)
		s.mu.Unlock()
	case types.SideTypeBuy:
		s.mu.Lock()
		s.CoveredPosition = s.CoveredPosition.Add(quantity.Neg())
		s.mu.Unlock()
	}
}

func (s *Strategy) runTradeRecover(ctx context.Context) {
	tradeScanInterval := s.RecoverTradeScanPeriod.Duration()
	if tradeScanInterval == 0 {
		tradeScanInterval = 30 * time.Minute
	}

	tradeScanOverlapBufferPeriod := 5 * time.Minute

	tradeScanTicker := time.NewTicker(tradeScanInterval)
	defer tradeScanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-tradeScanTicker.C:
			log.Infof("scanning trades from %s ago...", tradeScanInterval)

			startTime := time.Now().Add(-tradeScanInterval).Add(-tradeScanOverlapBufferPeriod)

			if err := s.HedgeOrderExecutor.TradeCollector().Recover(ctx, s.hedgeSession.Exchange.(types.ExchangeTradeHistoryService), s.HedgeSymbol, startTime); err != nil {
				log.WithError(err).Errorf("query trades error")
			}

			if err := s.MakerOrderExecutor.TradeCollector().Recover(ctx, s.makerSession.Exchange.(types.ExchangeTradeHistoryService), s.Symbol, startTime); err != nil {
				log.WithError(err).Errorf("query trades error")
			}
		}
	}
}

func (s *Strategy) generateMakerOrders(
	pricingBook *types.StreamOrderBook,
	maxLayer int,
	availableBase, availableQuote fixedpoint.Value,
) ([]types.SubmitOrder, error) {
	_, _, hasPrice := pricingBook.BestBidAndAsk()
	if !hasPrice {
		return nil, nil
	}

	var submitOrders []types.SubmitOrder
	var accumulatedBidQuantity = fixedpoint.Zero
	var accumulatedAskQuantity = fixedpoint.Zero
	var accumulatedBidQuoteQuantity = fixedpoint.Zero

	// copy the pricing book because during the generation the book data could change
	dupPricingBook := pricingBook.Copy()

	log.Infof("pricingBook: \n\tbids: %+v \n\tasks: %+v",
		dupPricingBook.SideBook(types.SideTypeBuy),
		dupPricingBook.SideBook(types.SideTypeSell))

	if maxLayer == 0 || maxLayer > s.NumLayers {
		maxLayer = s.NumLayers
	}

	var availableBalances = map[types.SideType]fixedpoint.Value{
		types.SideTypeBuy:  availableQuote,
		types.SideTypeSell: availableBase,
	}

	for _, side := range []types.SideType{types.SideTypeBuy, types.SideTypeSell} {
		sideBook := dupPricingBook.SideBook(side)
		if sideBook.Len() == 0 {
			log.Warnf("orderbook %s side is empty", side)
			continue
		}

		availableSideBalance, ok := availableBalances[side]
		if !ok {
			log.Warnf("no available balance for side %s side", side)
			continue
		}

	layerLoop:
		for i := 1; i <= maxLayer; i++ {
			// simple break, we need to check the market minNotional and minQuantity later
			if !availableSideBalance.Eq(fixedpoint.PosInf) {
				if availableSideBalance.IsZero() || availableSideBalance.Sign() < 0 {
					break layerLoop
				}
			}

			requiredDepthFloat, err := s.DepthScale.Scale(i)
			if err != nil {
				return nil, errors.Wrapf(err, "depthScale scale error")
			}

			// requiredDepth is the required depth in quote currency
			requiredDepth := fixedpoint.NewFromFloat(requiredDepthFloat)

			index := sideBook.IndexByQuoteVolumeDepth(requiredDepth)

			pvs := types.PriceVolumeSlice{}
			if index == -1 {
				pvs = sideBook[:]
			} else {
				pvs = sideBook[0 : index+1]
			}

			if len(pvs) == 0 {
				continue
			}

			log.Infof("side: %s required depth: %f, pvs: %+v", side, requiredDepth.Float64(), pvs)

			depthPrice := pvs.AverageDepthPriceByQuote(fixedpoint.Zero, 0)

			switch side {
			case types.SideTypeBuy:
				if s.BidMargin.Sign() > 0 {
					depthPrice = depthPrice.Mul(fixedpoint.One.Sub(s.BidMargin))
				}

				depthPrice = depthPrice.Round(s.makerMarket.PricePrecision+1, fixedpoint.Down)

			case types.SideTypeSell:
				if s.AskMargin.Sign() > 0 {
					depthPrice = depthPrice.Mul(fixedpoint.One.Add(s.AskMargin))
				}

				depthPrice = depthPrice.Round(s.makerMarket.PricePrecision+1, fixedpoint.Up)
			}

			depthPrice = s.makerMarket.TruncatePrice(depthPrice)

			quantity := requiredDepth.Div(depthPrice)
			quantity = s.makerMarket.TruncateQuantity(quantity)
			log.Infof("side: %s required depth: %f price: %f quantity: %f", side, requiredDepth.Float64(), depthPrice.Float64(), quantity.Float64())

			switch side {
			case types.SideTypeBuy:
				quantity = quantity.Sub(accumulatedBidQuantity)

				accumulatedBidQuantity = accumulatedBidQuantity.Add(quantity)
				quoteQuantity := fixedpoint.Mul(quantity, depthPrice)
				quoteQuantity = quoteQuantity.Round(s.makerMarket.PricePrecision, fixedpoint.Up)

				if !availableSideBalance.Eq(fixedpoint.PosInf) && availableSideBalance.Compare(quoteQuantity) <= 0 {
					quoteQuantity = availableSideBalance
					quantity = quoteQuantity.Div(depthPrice).Round(s.makerMarket.PricePrecision, fixedpoint.Down)
				}

				if quantity.Compare(s.makerMarket.MinQuantity) <= 0 || quoteQuantity.Compare(s.makerMarket.MinNotional) <= 0 {
					break layerLoop
				}

				availableSideBalance = availableSideBalance.Sub(quoteQuantity)

				accumulatedBidQuoteQuantity = accumulatedBidQuoteQuantity.Add(quoteQuantity)

			case types.SideTypeSell:
				quantity = quantity.Sub(accumulatedAskQuantity)
				quoteQuantity := quantity.Mul(depthPrice)

				// balance check
				if !availableSideBalance.Eq(fixedpoint.PosInf) && availableSideBalance.Compare(quantity) <= 0 {
					break layerLoop
				}

				if quantity.Compare(s.makerMarket.MinQuantity) <= 0 || quoteQuantity.Compare(s.makerMarket.MinNotional) <= 0 {
					break layerLoop
				}

				availableSideBalance = availableSideBalance.Sub(quantity)

				accumulatedAskQuantity = accumulatedAskQuantity.Add(quantity)
			}

			submitOrders = append(submitOrders, types.SubmitOrder{
				Symbol:   s.makerMarket.Symbol,
				Type:     types.OrderTypeLimitMaker,
				Market:   s.makerMarket,
				Side:     side,
				Price:    depthPrice,
				Quantity: quantity,
			})
		}
	}

	return submitOrders, nil
}

func (s *Strategy) partiallyCancelOrders(ctx context.Context, maxLayer int) error {
	buyOrders, sellOrders := s.MakerOrderExecutor.ActiveMakerOrders().Orders().SeparateBySide()
	buyOrders = types.SortOrdersByPrice(buyOrders, true)
	sellOrders = types.SortOrdersByPrice(sellOrders, false)

	buyOrdersToCancel := buyOrders[0:min(maxLayer, len(buyOrders))]
	sellOrdersToCancel := sellOrders[0:min(maxLayer, len(sellOrders))]

	err1 := s.MakerOrderExecutor.GracefulCancel(ctx, buyOrdersToCancel...)
	err2 := s.MakerOrderExecutor.GracefulCancel(ctx, sellOrdersToCancel...)
	return stderrors.Join(err1, err2)
}

func (s *Strategy) updateQuote(ctx context.Context, maxLayer int) {
	if maxLayer == 0 {
		if err := s.MakerOrderExecutor.GracefulCancel(ctx); err != nil {
			log.WithError(err).Warnf("there are some %s orders not canceled, skipping placing maker orders", s.Symbol)
			s.MakerOrderExecutor.ActiveMakerOrders().Print()
			return
		}
	} else {
		if err := s.partiallyCancelOrders(ctx, maxLayer); err != nil {
			log.WithError(err).Warnf("%s partial order cancel failed", s.Symbol)
			return
		}
	}

	numOfMakerOrders := s.MakerOrderExecutor.ActiveMakerOrders().NumOfOrders()
	if numOfMakerOrders > 0 {
		log.Warnf("maker orders are not all canceled")
		return
	}

	bestBid, bestAsk, hasPrice := s.pricingBook.BestBidAndAsk()
	if !hasPrice {
		return
	}

	bestBidPrice := bestBid.Price
	bestAskPrice := bestAsk.Price
	log.Infof("%s book ticker: best ask / best bid = %v / %v", s.HedgeSymbol, bestAskPrice, bestBidPrice)

	s.lastPrice = bestBidPrice.Add(bestAskPrice).Div(Two)

	bookLastUpdateTime := s.pricingBook.LastUpdateTime()

	if _, err := s.bidPriceHeartBeat.Update(bestBid); err != nil {
		log.WithError(err).Warnf("quote update error, %s price not updating, order book last update: %s ago",
			s.Symbol,
			time.Since(bookLastUpdateTime))
	}

	if _, err := s.askPriceHeartBeat.Update(bestAsk); err != nil {
		log.WithError(err).Warnf("quote update error, %s price not updating, order book last update: %s ago",
			s.Symbol,
			time.Since(bookLastUpdateTime))
	}

	balances, err := s.MakerOrderExecutor.Session().Exchange.QueryAccountBalances(ctx)
	if err != nil {
		log.WithError(err).Errorf("balance query error")
		return
	}

	log.Infof("balances: %+v", balances.NotZero())

	quoteBalance, ok := balances[s.makerMarket.QuoteCurrency]
	if !ok {
		return
	}

	baseBalance, ok := balances[s.makerMarket.BaseCurrency]
	if !ok {
		return
	}

	log.Infof("quote balance: %s, base balance: %s", quoteBalance, baseBalance)

	submitOrders, err := s.generateMakerOrders(s.pricingBook, maxLayer, baseBalance.Available, quoteBalance.Available)
	if err != nil {
		log.WithError(err).Errorf("generate order error")
		return
	}

	if len(submitOrders) == 0 {
		log.Warnf("no orders are generated")
		return
	}

	_, err = s.MakerOrderExecutor.SubmitOrders(ctx, submitOrders...)
	if err != nil {
		log.WithError(err).Errorf("order error: %s", err.Error())
		return
	}
}

func (s *Strategy) cleanUpOpenOrders(ctx context.Context, session *bbgo.ExchangeSession) error {
	openOrders, err := retry.QueryOpenOrdersUntilSuccessful(ctx, session.Exchange, s.Symbol)
	if err != nil {
		return err
	}

	if len(openOrders) == 0 {
		return nil
	}

	log.Infof("found existing open orders:")
	types.OrderSlice(openOrders).Print()

	return session.Exchange.CancelOrders(ctx, openOrders...)
}

func selectSessions2(
	sessions map[string]*bbgo.ExchangeSession, n1, n2 string,
) (s1, s2 *bbgo.ExchangeSession, err error) {
	for _, n := range []string{n1, n2} {
		if _, ok := sessions[n]; !ok {
			return nil, nil, fmt.Errorf("session %s is not defined", n)
		}
	}

	s1 = sessions[n1]
	s2 = sessions[n2]
	return s1, s2, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func bindAuthSignal(ctx context.Context, stream types.Stream, c chan<- struct{}) {
	stream.OnAuth(func() {
		select {
		case <-ctx.Done():
			return
		case c <- struct{}{}:
		default:
		}
	})
}
