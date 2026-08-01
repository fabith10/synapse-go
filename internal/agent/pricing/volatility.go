package pricing

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// extractATMVol extracts ATM implied volatility from vol surface outputs.
func extractATMVol(rawJSON string) float64 {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
		return 0
	}
	// Case 1: {"vol_surface": [{"tenor": "30d", "atm_vol": 0.385}]}
	if vs, ok := m["vol_surface"].([]interface{}); ok && len(vs) > 0 {
		for _, item := range vs {
			if vMap, ok := item.(map[string]interface{}); ok {
				if tenor, _ := vMap["tenor"].(string); tenor == "30d" || tenor == "30-day" {
					if atm, ok := vMap["atm_vol"].(float64); ok {
						return atm
					}
				}
			}
		}
		if vMap, ok := vs[0].(map[string]interface{}); ok {
			if atm, ok := vMap["atm_vol"].(float64); ok {
				return atm
			}
		}
	}
	// Case 2: {"atm_vol_30d": 0.380}
	if atm, ok := m["atm_vol_30d"].(float64); ok {
		return atm
	}
	return 0
}

// extractIVFromOptions extracts or calculates implied volatility from options chain responses using Newton-Raphson BS inversion.
func extractIVFromOptions(rawJSON string, spot float64) float64 {
	if rawJSON == "" || spot <= 0 {
		return 0
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return 0
	}

	var optionsList []interface{}
	if opts, ok := data["options"].([]interface{}); ok {
		optionsList = opts
	} else if contracts, ok := data["contracts"].([]interface{}); ok {
		optionsList = contracts
	} else if list, ok := data["chain"].([]interface{}); ok {
		optionsList = list
	} else {
		var slice []interface{}
		if err := json.Unmarshal([]byte(rawJSON), &slice); err == nil {
			optionsList = slice
		}
	}

	if len(optionsList) == 0 {
		if iv, ok := data["implied_volatility"].(float64); ok && iv > 0 {
			return iv
		}
		if iv, ok := data["atm_vol"].(float64); ok && iv > 0 {
			return iv
		}
		return 0
	}

	var extractedIVs []float64
	r := 0.02 // Assumed risk-free interest rate (2%)

	for _, item := range optionsList {
		optMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		ivVal := 0.0
		if iv, ok := optMap["implied_volatility"].(float64); ok && iv > 0 {
			ivVal = iv
		} else if iv, ok := optMap["iv"].(float64); ok && iv > 0 {
			ivVal = iv
		}

		strikeVal := 0.0
		if s, ok := optMap["strike"].(float64); ok {
			strikeVal = s
		}

		days := 30.0
		if dStr, ok := optMap["expiration"].(string); ok {
			if strings.HasSuffix(dStr, "d") {
				if d, err := strconv.ParseFloat(strings.TrimSuffix(dStr, "d"), 64); err == nil && d > 0 {
					days = d
				}
			}
		}
		tYears := days / 365.0
		if tYears <= 0 {
			tYears = 30.0 / 365.0
		}

		diff := math.Abs(strikeVal - spot)

		if ivVal > 0 {
			// Extract ATM explicit IV (within 15% of spot)
			if strikeVal > 0 && diff/spot <= 0.15 {
				extractedIVs = append(extractedIVs, ivVal)
			}
		} else if strikeVal > 0 {
			// Dynamic Newton-Raphson Black-Scholes inversion
			optTypeStr, _ := optMap["type"].(string)
			isCall := strings.ToLower(optTypeStr) != "put"

			prem := 0.0
			if p, ok := optMap["last_price"].(float64); ok && p > 0 {
				prem = p
			} else if p, ok := optMap["call_premium"].(float64); ok && p > 0 {
				prem = p
			} else if p, ok := optMap["put_premium"].(float64); ok && p > 0 {
				prem = p
			} else if p, ok := optMap["ask"].(float64); ok && p > 0 {
				bid, _ := optMap["bid"].(float64)
				prem = (bid + p) / 2.0
			}

			if prem > 0 && diff/spot <= 0.15 {
				calculatedIV := ImpliedVolatilityBS(isCall, spot, strikeVal, tYears, r, prem)
				if calculatedIV > 0.01 && calculatedIV < 5.0 {
					extractedIVs = append(extractedIVs, calculatedIV)
				}
			}
		}
	}

	if len(extractedIVs) > 0 {
		sum := 0.0
		for _, v := range extractedIVs {
			sum += v
		}
		return roundTo4(sum / float64(len(extractedIVs)))
	}

	return 0
}
