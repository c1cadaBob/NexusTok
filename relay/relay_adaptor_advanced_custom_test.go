package relay

import (
	"testing"

	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/relay/channel/advancedcustom"
	"github.com/stretchr/testify/require"
)

func TestGetAdaptorAdvancedCustom(t *testing.T) {
	adaptor := GetAdaptor(constant.APITypeAdvancedCustom)

	require.IsType(t, &advancedcustom.Adaptor{}, adaptor)
}
