package memory_test

import (
	"testing"

	"codeplat/internal/core"
	"codeplat/internal/store/memory"
	"codeplat/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) core.Store { return memory.New() })
}
