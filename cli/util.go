package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
)

func (c *CLI) keygen(args []string) error {
	count := 1
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return fmt.Errorf("invalid count: %s (must be a positive integer)", args[0])
		}
		count = n
	}

	for i := 0; i < count; i++ {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			return fmt.Errorf("failed to generate random key: %w", err)
		}
		fmt.Println(hex.EncodeToString(bytes))
	}

	return nil
}
