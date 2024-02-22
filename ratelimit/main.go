package ratelimit

// This is a goof-around and not a real code.

import (
	"context"
	"github.com/go-redis/redis/v8" // Example Redis client
)

type RedisTokenBucket struct {
	redisClient *redis.Client
	rate        int64 // Tokens per second
	burst       int64 // Maximum bucket size
}

func NewRedisTokenBucket(redisClient *redis.Client, rate, burst int64) *RedisTokenBucket {
	return &RedisTokenBucket{
		redisClient: redisClient,
		rate:        rate,
		burst:       burst,
	}
}

func (tb *RedisTokenBucket) Allow(ctx context.Context, key string) bool {
	// 1. Get the current state from Redis
	result, err := tb.redisClient.Eval(ctx, `
        local current_tokens = tonumber(redis.call('GET', KEYS[1]) or "0")
        local now = tonumber(redis.call('TIME')[1]) -- Seconds since Unix epoch 

        -- Calculate new tokens based on elapsed time
        local elapsed_time = now - tonumber(redis.call('HGET', KEYS[1], 'last_update') or now)  
        local new_tokens = math.min(current_tokens + elapsed_time * ARGV[1], ARGV[2]) 

        -- Try to consume a token 
        if new_tokens >= 1 then
            redis.call('HSET', KEYS[1], 'last_update', now)
            redis.call('DECR', KEYS[1]) 
            return 1 -- Request allowed
        else
            return 0 -- Request denied (rate limit exceeded)
        end
    `, []string{key}, tb.rate, tb.burst).Result()

	if err != nil {
		// Handle Redis errors
		return false
	}

	return result.(int64) == 1
}

func (tb *RedisTokenBucket) refillTokensIfNeeded(ctx context.Context, key string) {
	result, err := tb.redisClient.Eval(ctx, `
        local current_tokens = tonumber(redis.call('GET', KEYS[1]) or "0")
        local now = tonumber(redis.call('TIME')[1]) 
        local last_update = tonumber(redis.call('HGET', KEYS[1], 'last_update') or now)
        local elapsed_time = now - last_update

        -- Only refill if bucket wasn't already full
        if current_tokens < ARGV[1] then 
            local new_tokens = math.min(current_tokens + elapsed_time * ARGV[2], ARGV[1]) 
            redis.call('SET', KEYS[1], new_tokens)
            redis.call('HSET', KEYS[1], 'last_update', now)  
            return 1 -- Indicate refill occurred
        else
            return 0 -- No refill needed
        end
        `, []string{key}, tb.burst, tb.rate).Result()

	if err != nil {
		// Handle potential Redis errors
		return
	}

	// Optionally log if refill took place:
	if result.(int64) == 1 {
		// ... logging here ...
	}
}
