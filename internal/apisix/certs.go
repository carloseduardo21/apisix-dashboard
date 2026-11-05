package apisix

import (
	"crypto/x509"
	"encoding/pem"
)

// AnnotateCerts adds validity_start/validity_end (unix seconds) to SSL objects
// when APISIX does not provide them (recent versions stopped storing them),
// so the UI can warn about expiring certificates. Only the public certificate
// is parsed.
func AnnotateCerts(env any) {
	m, ok := env.(map[string]any)
	if !ok {
		return
	}
	if v, ok := m["value"].(map[string]any); ok {
		annotate(v)
	}
	if list, ok := m["list"].([]any); ok {
		for _, item := range list {
			if im, ok := item.(map[string]any); ok {
				if v, ok := im["value"].(map[string]any); ok {
					annotate(v)
				}
			}
		}
	}
}

func annotate(v map[string]any) {
	if _, ok := v["validity_end"]; ok {
		return
	}
	pemStr, _ := v["cert"].(string)
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	v["validity_start"] = cert.NotBefore.Unix()
	v["validity_end"] = cert.NotAfter.Unix()
}
