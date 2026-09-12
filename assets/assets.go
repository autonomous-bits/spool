package assets

import (
	_ "embed"
	"encoding/base64"
	"fmt"
)

//go:embed icon.png
var IconPNG []byte

// IconPNGDataURI contains the base64-encoded data URI for the icon.
var IconPNGDataURI = fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(IconPNG))
