package handler

import (
	"errors"
	"sync"
	"time"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
)

// workspace is the per-request scratch buffer borrowed from wsPool.
type workspace struct {
	body [8192]byte
	raw  fraud.RawRequest
	// knownMerchants backs the list during parse (avoids heap allocation).
	knownMerchants [8][]byte
}

var wsPool = sync.Pool{New: func() any { return new(workspace) }}

// parseBody fills dst from the JSON body in buf. The format is fixed:
// id, transaction, customer, merchant, terminal, last_transaction.
// Allocates only for MCC (4-byte string) and merchant/known-merchant IDs.
func parseBody(buf []byte, dst *fraud.RawRequest) error {
	s := scanner{buf: buf}
	// Expect opening `{`
	if !s.expectByte('{') {
		return errors.New("expected {")
	}

	// Scratch storage for known_merchants (slice of buf slices).
	var kmBuf [8][]byte
	var kmCount int
	var merchantIDBytes []byte

	for !s.done() {
		key := s.readKey()
		if len(key) == 0 {
			break
		}

		switch string(key) {
		case "id":
			s.skipValue()

		case "transaction":
			if !s.expectByte('{') {
				return errors.New("expected { for transaction")
			}
			for !s.done() {
				tk := s.readKey()
				if len(tk) == 0 {
					break
				}
				switch string(tk) {
				case "amount":
					dst.TxAmount = s.readFloat()
				case "installments":
					dst.TxInstallments = int(s.readInt())
				case "requested_at":
					dst.TxTime = s.readTimestamp()
				default:
					s.skipValue()
				}
			}
			s.expectByte('}') // consume closing }

		case "customer":
			if !s.expectByte('{') {
				return errors.New("expected { for customer")
			}
			for !s.done() {
				ck := s.readKey()
				if len(ck) == 0 {
					break
				}
				switch string(ck) {
				case "avg_amount":
					dst.CustAvgAmount = s.readFloat()
				case "tx_count_24h":
					dst.CustTxCount24h = int(s.readInt())
				case "known_merchants":
					kmCount = s.readStringArray(kmBuf[:])
				default:
					s.skipValue()
				}
			}
			s.expectByte('}') // consume closing }

		case "merchant":
			if !s.expectByte('{') {
				return errors.New("expected { for merchant")
			}
			for !s.done() {
				mk := s.readKey()
				if len(mk) == 0 {
					break
				}
				switch string(mk) {
				case "id":
					merchantIDBytes = s.readStringBytes()
				case "mcc":
					dst.MCC = string(s.readStringBytes())
				case "avg_amount":
					dst.MerchantAvg = s.readFloat()
				default:
					s.skipValue()
				}
			}
			s.expectByte('}') // consume closing }

		case "terminal":
			if !s.expectByte('{') {
				return errors.New("expected { for terminal")
			}
			for !s.done() {
				tk := s.readKey()
				if len(tk) == 0 {
					break
				}
				switch string(tk) {
				case "is_online":
					dst.TermIsOnline = s.readBool()
				case "card_present":
					dst.TermCardPresent = s.readBool()
				case "km_from_home":
					dst.TermKmFromHome = s.readFloat()
				default:
					s.skipValue()
				}
			}
			s.expectByte('}') // consume closing }

		case "last_transaction":
			s.skipWS()
			if s.peekByte() == 'n' {
				s.advance(4) // null
				dst.HasLastTx = false
			} else if s.expectByte('{') {
				dst.HasLastTx = true
				for !s.done() {
					lk := s.readKey()
					if len(lk) == 0 {
						break
					}
					switch string(lk) {
					case "timestamp":
						dst.LastTxTime = s.readTimestamp()
					case "km_from_current":
						dst.LastKmFromCurrent = s.readFloat()
					default:
						s.skipValue()
					}
				}
				s.expectByte('}') // consume closing }
			}

		default:
			s.skipValue()
		}
	}

	// Determine unknown_merchant: merchant ID not in known list.
	dst.UnknownMerchant = true
	for i := 0; i < kmCount; i++ {
		if bytesEqual(kmBuf[i], merchantIDBytes) {
			dst.UnknownMerchant = false
			break
		}
	}

	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// scanner is a zero-allocation JSON cursor over a byte slice.
type scanner struct {
	buf []byte
	pos int
}

func (s *scanner) done() bool {
	s.skipWS()
	return s.pos >= len(s.buf) || s.buf[s.pos] == '}' || s.buf[s.pos] == ']'
}

func (s *scanner) skipWS() {
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s.pos++
	}
}

func (s *scanner) peekByte() byte {
	if s.pos < len(s.buf) {
		return s.buf[s.pos]
	}
	return 0
}

func (s *scanner) advance(n int) {
	s.pos += n
	if s.pos > len(s.buf) {
		s.pos = len(s.buf)
	}
}

func (s *scanner) expectByte(b byte) bool {
	s.skipWS()
	if s.pos < len(s.buf) && s.buf[s.pos] == b {
		s.pos++
		return true
	}
	return false
}

// readKey reads a JSON string key and the following colon; returns slice into buf.
func (s *scanner) readKey() []byte {
	s.skipWS()
	// skip comma if present
	if s.pos < len(s.buf) && s.buf[s.pos] == ',' {
		s.pos++
		s.skipWS()
	}
	if s.pos >= len(s.buf) || s.buf[s.pos] != '"' {
		return nil
	}
	key := s.readStringBytes()
	s.skipWS()
	if s.pos < len(s.buf) && s.buf[s.pos] == ':' {
		s.pos++
	}
	return key
}

// readStringBytes reads a JSON string (without unescaping) and returns a slice into buf.
func (s *scanner) readStringBytes() []byte {
	s.skipWS()
	if s.pos >= len(s.buf) || s.buf[s.pos] != '"' {
		return nil
	}
	s.pos++ // skip opening "
	start := s.pos
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		if c == '"' {
			end := s.pos
			s.pos++ // skip closing "
			return s.buf[start:end]
		}
		if c == '\\' {
			s.pos++ // skip escape
		}
		s.pos++
	}
	return nil
}

// readStringArray reads a JSON array of strings into dst; returns count.
func (s *scanner) readStringArray(dst [][]byte) int {
	s.skipWS()
	if !s.expectByte('[') {
		return 0
	}
	count := 0
	closed := false
	for count < len(dst) {
		s.skipWS()
		if s.pos < len(s.buf) && s.buf[s.pos] == ']' {
			s.pos++
			closed = true
			break
		}
		if s.pos < len(s.buf) && s.buf[s.pos] == ',' {
			s.pos++
		}
		v := s.readStringBytes()
		if v == nil {
			break
		}
		dst[count] = v
		count++
	}
	// drain remaining elements only if closing ] was not yet consumed
	if !closed {
		for {
			s.skipWS()
			if s.pos >= len(s.buf) || s.buf[s.pos] == ']' {
				s.pos++
				break
			}
			if s.buf[s.pos] == ',' {
				s.pos++
			}
			s.skipValue()
		}
	}
	return count
}

// readFloat parses a JSON number (int or decimal) without strconv.ParseFloat.
func (s *scanner) readFloat() float64 {
	s.skipWS()
	neg := false
	if s.pos < len(s.buf) && s.buf[s.pos] == '-' {
		neg = true
		s.pos++
	}
	var intPart int64
	for s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '9' {
		intPart = intPart*10 + int64(s.buf[s.pos]-'0')
		s.pos++
	}
	var fracPart float64
	if s.pos < len(s.buf) && s.buf[s.pos] == '.' {
		s.pos++
		frac := float64(1)
		for s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '9' {
			frac /= 10
			fracPart += float64(s.buf[s.pos]-'0') * frac
			s.pos++
		}
	}
	// skip exponent if present (shouldn't appear in test data, but be safe)
	if s.pos < len(s.buf) && (s.buf[s.pos] == 'e' || s.buf[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.buf) && (s.buf[s.pos] == '+' || s.buf[s.pos] == '-') {
			s.pos++
		}
		for s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '9' {
			s.pos++
		}
	}
	result := float64(intPart) + fracPart
	if neg {
		result = -result
	}
	return result
}

// readInt parses a JSON integer.
func (s *scanner) readInt() int64 {
	s.skipWS()
	neg := false
	if s.pos < len(s.buf) && s.buf[s.pos] == '-' {
		neg = true
		s.pos++
	}
	var v int64
	for s.pos < len(s.buf) && s.buf[s.pos] >= '0' && s.buf[s.pos] <= '9' {
		v = v*10 + int64(s.buf[s.pos]-'0')
		s.pos++
	}
	if neg {
		return -v
	}
	return v
}

// readBool parses a JSON boolean.
func (s *scanner) readBool() bool {
	s.skipWS()
	if s.pos+4 <= len(s.buf) && s.buf[s.pos] == 't' {
		s.pos += 4 // true
		return true
	}
	s.pos += 5 // false
	return false
}

// readTimestamp parses a fixed-format RFC3339 timestamp "YYYY-MM-DDTHH:mm:ssZ".
// Does not support fractional seconds or non-UTC offsets.
func (s *scanner) readTimestamp() time.Time {
	s.skipWS()
	if s.pos >= len(s.buf) || s.buf[s.pos] != '"' {
		return time.Time{}
	}
	s.pos++ // skip opening "
	b := s.buf[s.pos:]
	if len(b) < 20 {
		return time.Time{}
	}
	year := int(b[0]-'0')*1000 + int(b[1]-'0')*100 + int(b[2]-'0')*10 + int(b[3]-'0')
	month := time.Month(int(b[5]-'0')*10 + int(b[6]-'0'))
	day := int(b[8]-'0')*10 + int(b[9]-'0')
	hour := int(b[11]-'0')*10 + int(b[12]-'0')
	min := int(b[14]-'0')*10 + int(b[15]-'0')
	sec := int(b[17]-'0')*10 + int(b[18]-'0')
	s.pos += 21 // 20 chars + closing "
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

// skipValue skips a single JSON value (string, number, bool, null, object, array).
func (s *scanner) skipValue() {
	s.skipWS()
	if s.pos >= len(s.buf) {
		return
	}
	switch s.buf[s.pos] {
	case '"':
		s.readStringBytes()
	case '{':
		s.pos++
		depth := 1
		for s.pos < len(s.buf) && depth > 0 {
			switch s.buf[s.pos] {
			case '{':
				depth++
			case '}':
				depth--
			case '"':
				s.readStringBytes()
				continue
			}
			s.pos++
		}
	case '[':
		s.pos++
		depth := 1
		for s.pos < len(s.buf) && depth > 0 {
			switch s.buf[s.pos] {
			case '[':
				depth++
			case ']':
				depth--
			case '"':
				s.readStringBytes()
				continue
			}
			s.pos++
		}
	case 't':
		s.pos += 4 // true
	case 'f':
		s.pos += 5 // false
	case 'n':
		s.pos += 4 // null
	default:
		// number: consume until non-number char
		for s.pos < len(s.buf) {
			c := s.buf[s.pos]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\n' || c == '\r' || c == '\t' {
				break
			}
			s.pos++
		}
	}
}
