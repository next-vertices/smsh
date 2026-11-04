package service

import (
	"context"
	"math"
	"sort"
	"sync"

	"smsh/internal/ovh"
	"smsh/internal/sms"
)

// billingCountry : pays d'achat des crédits utilisé pour les tarifs publics.
const billingCountry = "fr"

// CostInput est la saisie dont on estime le coût.
type CostInput struct {
	Message    string `json:"message"`
	Recipients string `json:"recipients"`
	SenderType string `json:"senderType"` // type de l'expéditeur choisi ; vide = shortcode
	StopClause bool   `json:"stopClause"`
}

// CostLine est le coût pour un pays de destination.
type CostLine struct {
	Country       string  `json:"country"`
	Recipients    int     `json:"recipients"`
	CreditsPerSMS float64 `json:"creditsPerSms"`
	Credits       float64 `json:"credits"`
	PriceMin      float64 `json:"priceMin"` // prix public HT selon le pack acheté
	PriceMax      float64 `json:"priceMax"`
}

// CostEstimate est l'estimation complète d'un envoi.
type CostEstimate struct {
	Characters   int        `json:"characters"`
	Encoding     string     `json:"encoding"`
	PerPart      int        `json:"perPart"`
	Parts        int        `json:"parts"`
	Recipients   int        `json:"recipients"`
	Lines        []CostLine `json:"lines"`
	TotalCredits float64    `json:"totalCredits"`
	PriceMin     float64    `json:"priceMin"`
	PriceMax     float64    `json:"priceMax"`
	Currency     string     `json:"currency"`
	Unknown      []string   `json:"unknown"` // numéros dont le pays est inconnu (non comptés)
	Warning      string     `json:"warning"`
}

type rateEntry struct {
	rate  ovh.DestinationRate
	tiers []ovh.PackTier
}

// rateCache mémorise les tarifs par pays pour la durée de vie du Service.
type rateCache struct {
	mu sync.Mutex
	m  map[string]rateEntry
}

func round(v float64, dec int) float64 {
	p := math.Pow(10, float64(dec))
	return math.Round(v*p) / p
}

func (s *Service) rateFor(ctx context.Context, cl ovh.Client, country string) (rateEntry, error) {
	s.rates.mu.Lock()
	e, ok := s.rates.m[country]
	s.rates.mu.Unlock()
	if ok {
		return e, nil
	}
	r, err := cl.DestinationRate(ctx, billingCountry, country)
	if err != nil {
		return rateEntry{}, err
	}
	tiers, err := cl.PackTiers(ctx, billingCountry, country)
	if err != nil || len(tiers) == 0 {
		tiers = []ovh.PackTier{{PricePerSMS: r.PricePerSMS}} // repli : tarif de la destination
	}
	e = rateEntry{rate: r, tiers: tiers}
	s.rates.mu.Lock()
	if s.rates.m == nil {
		s.rates.m = map[string]rateEntry{}
	}
	s.rates.m[country] = e
	s.rates.mu.Unlock()
	return e, nil
}

// EstimateCost calcule le coût d'un envoi : segments via POST /sms/estimate,
// tarif par pays via /sms/rates/destinations et /sms/rates/packs.
func (s *Service) EstimateCost(ctx context.Context, in CostInput) (CostEstimate, error) {
	est := CostEstimate{Lines: []CostLine{}, Unknown: []string{}}
	rcpt := sms.ParseReceivers(in.Recipients)
	if in.Message == "" || len(rcpt.Valid) == 0 {
		return est, nil
	}
	a, sec, err := s.Creds.Active()
	if err != nil {
		return est, err
	}
	cl, err := s.NewClient(toOVH(a, sec))
	if err != nil {
		return est, err
	}

	senderType := in.SenderType
	if senderType == "" {
		senderType = "shortcode"
	}
	if me, err := cl.EstimateMessage(ctx, in.Message, in.StopClause, senderType); err == nil && me.Parts > 0 {
		est.Characters, est.Encoding, est.PerPart, est.Parts = me.Characters, me.Encoding, me.PerPart, me.Parts
	} else {
		// Repli sur le calcul local si l'endpoint est refusé (droit POST /sms/estimate manquant…).
		an := sms.Analyze(in.Message)
		enc := "7bits"
		if an.Encoding == sms.UCS2 {
			enc = "unicode"
		}
		est.Characters, est.Encoding, est.PerPart, est.Parts = an.Chars, enc, an.PerSegment, an.Segments
		est.Warning = "Segments calculés localement (estimation OVH indisponible)."
		if err != nil {
			est.Warning = "Segments calculés localement : " + err.Error()
		}
	}

	byCountry := map[string]int{}
	for _, n := range rcpt.Valid {
		if c := sms.CountryOf(n); c != "" {
			byCountry[c]++
		} else {
			est.Unknown = append(est.Unknown, n)
		}
	}
	countries := make([]string, 0, len(byCountry))
	for c := range byCountry {
		countries = append(countries, c)
	}
	sort.Strings(countries)

	for _, c := range countries {
		e, err := s.rateFor(ctx, cl, c)
		if err != nil {
			return est, err
		}
		count := float64(byCountry[c])
		lo, hi := e.tiers[0].PricePerSMS, e.tiers[0].PricePerSMS
		for _, t := range e.tiers {
			lo, hi = math.Min(lo, t.PricePerSMS), math.Max(hi, t.PricePerSMS)
		}
		units := count * float64(est.Parts)
		line := CostLine{
			Country: c, Recipients: byCountry[c], CreditsPerSMS: e.rate.CreditsPerSMS,
			Credits:  round(units*e.rate.CreditsPerSMS, 3),
			PriceMin: round(units*lo, 3), PriceMax: round(units*hi, 3),
		}
		est.Lines = append(est.Lines, line)
		est.Recipients += byCountry[c]
		est.TotalCredits += line.Credits
		est.PriceMin += line.PriceMin
		est.PriceMax += line.PriceMax
		est.Currency = e.rate.Currency
	}
	est.TotalCredits = round(est.TotalCredits, 3)
	est.PriceMin, est.PriceMax = round(est.PriceMin, 3), round(est.PriceMax, 3)
	return est, nil
}
