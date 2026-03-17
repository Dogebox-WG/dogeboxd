package utils

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForLocalIPReturnsImmediatelyWhenReady(t *testing.T) {
	var calls int
	var sleepCalls int

	expectedIP := net.IPv4(192, 168, 64, 2)
	ip, err := waitForLocalIP(func() (net.IP, error) {
		calls++
		return expectedIP, nil
	}, 3, time.Second, func(time.Duration) {
		sleepCalls++
	})

	require.NoError(t, err)
	assert.True(t, ip.Equal(expectedIP))
	assert.Equal(t, 1, calls)
	assert.Equal(t, 0, sleepCalls)
}

func TestWaitForLocalIPRetriesUntilReady(t *testing.T) {
	var calls int
	var sleepCalls int

	expectedIP := net.IPv4(192, 168, 64, 2)
	ip, err := waitForLocalIP(func() (net.IP, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("network is unreachable")
		}

		return expectedIP, nil
	}, 5, time.Second, func(time.Duration) {
		sleepCalls++
	})

	require.NoError(t, err)
	assert.True(t, ip.Equal(expectedIP))
	assert.Equal(t, 3, calls)
	assert.Equal(t, 2, sleepCalls)
}

func TestWaitForLocalIPFailsAfterAttempts(t *testing.T) {
	var calls int
	var sleepCalls int

	_, err := waitForLocalIP(func() (net.IP, error) {
		calls++
		return nil, errors.New("network is unreachable")
	}, 3, time.Second, func(time.Duration) {
		sleepCalls++
	})

	require.Error(t, err)
	assert.EqualError(t, err, "after 3 attempts, last error: network is unreachable")
	assert.Equal(t, 3, calls)
	assert.Equal(t, 2, sleepCalls)
}
