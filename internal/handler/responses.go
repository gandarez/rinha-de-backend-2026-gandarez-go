package handler

// responses holds the 6 possible JSON response bodies indexed by fraud neighbor count (0..5).
// With TopK=5 and FraudThreshold=0.6, fraudCount>=3 → denied.
// Pre-encoding eliminates json.Marshal reflection on every request.
var responses = [6][]byte{
	[]byte(`{"approved":true,"fraud_score":0}`),
	[]byte(`{"approved":true,"fraud_score":0.2}`),
	[]byte(`{"approved":true,"fraud_score":0.4}`),
	[]byte(`{"approved":false,"fraud_score":0.6}`),
	[]byte(`{"approved":false,"fraud_score":0.8}`),
	[]byte(`{"approved":false,"fraud_score":1}`),
}
