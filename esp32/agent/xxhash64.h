// xxhash64.h — Canonical XXH64 (Yan Collet, public domain) for ESP32-C3.
// Produces identical output to Go's github.com/cespare/xxhash/v2 Sum64().
// Seed is always 0.
#ifndef XXHASH64_H
#define XXHASH64_H

#include <stdint.h>
#include <string.h>
#include <stdio.h>

static const uint64_t XXH_PRIME64_1 = 0x9E3779B185EBCA87ULL;
static const uint64_t XXH_PRIME64_2 = 0x14DEF9DEA2F79CD5ULL;
static const uint64_t XXH_PRIME64_3 = 0x165667B19E3779F9ULL;
static const uint64_t XXH_PRIME64_4 = 0x85EBCA77C2B2AE63ULL;
static const uint64_t XXH_PRIME64_5 = 0x27D4EB2F165667C5ULL;

static inline uint64_t xxh_rotl64(uint64_t x, int r) {
    return (x << r) | (x >> (64 - r));
}

// Little-endian 64-bit read (ESP32-C3 is LE RISC-V).
static inline uint64_t xxh_read64le(const uint8_t *p) {
    uint64_t v;
    memcpy(&v, p, 8);
    return v;
}

// Little-endian 32-bit read.
static inline uint32_t xxh_read32le(const uint8_t *p) {
    uint32_t v;
    memcpy(&v, p, 4);
    return v;
}

static inline uint64_t xxh_round(uint64_t acc, uint64_t input) {
    acc += input * XXH_PRIME64_2;
    acc = xxh_rotl64(acc, 31);
    acc *= XXH_PRIME64_1;
    return acc;
}

static inline uint64_t xxh_mergeRound(uint64_t acc, uint64_t val) {
    val = xxh_round(0, val);
    acc ^= val;
    acc = acc * XXH_PRIME64_1 + XXH_PRIME64_4;
    return acc;
}

// XXH64 hash for arbitrary-length input, seed = 0.
static uint64_t xxhash64(const uint8_t *input, size_t len) {
    const uint8_t *p = input;
    const uint8_t *end = input + len;
    uint64_t h64;

    if (len >= 32) {
        const uint8_t *limit = end - 32;
        uint64_t v1 = 0 + XXH_PRIME64_1 + XXH_PRIME64_2;
        uint64_t v2 = 0 + XXH_PRIME64_2;
        uint64_t v3 = 0;
        uint64_t v4 = 0 - XXH_PRIME64_1;

        do {
            v1 = xxh_round(v1, xxh_read64le(p));      p += 8;
            v2 = xxh_round(v2, xxh_read64le(p));      p += 8;
            v3 = xxh_round(v3, xxh_read64le(p));      p += 8;
            v4 = xxh_round(v4, xxh_read64le(p));      p += 8;
        } while (p <= limit);

        h64 = xxh_rotl64(v1, 1) + xxh_rotl64(v2, 7) +
              xxh_rotl64(v3, 12) + xxh_rotl64(v4, 18);
        h64 = xxh_mergeRound(h64, v1);
        h64 = xxh_mergeRound(h64, v2);
        h64 = xxh_mergeRound(h64, v3);
        h64 = xxh_mergeRound(h64, v4);
    } else {
        h64 = 0 + XXH_PRIME64_5;
    }

    h64 += (uint64_t)len;

    // Process remaining 8-byte chunks.
    while (p + 8 <= end) {
        uint64_t k1 = xxh_read64le(p);
        k1 *= XXH_PRIME64_2;
        k1 = xxh_rotl64(k1, 31);
        k1 *= XXH_PRIME64_1;
        h64 ^= k1;
        h64 = xxh_rotl64(h64, 27) * XXH_PRIME64_1 + XXH_PRIME64_4;
        p += 8;
    }

    // Process remaining 4-byte chunk.
    if (p + 4 <= end) {
        h64 ^= (uint64_t)xxh_read32le(p) * XXH_PRIME64_1;
        h64 = xxh_rotl64(h64, 23) * XXH_PRIME64_2 + XXH_PRIME64_3;
        p += 4;
    }

    // Process remaining bytes.
    while (p < end) {
        h64 ^= (uint64_t)(*p) * XXH_PRIME64_5;
        h64 = xxh_rotl64(h64, 11) * XXH_PRIME64_1;
        p++;
    }

    // Avalanche.
    h64 ^= h64 >> 33;
    h64 *= XXH_PRIME64_2;
    h64 ^= h64 >> 29;
    h64 *= XXH_PRIME64_3;
    h64 ^= h64 >> 32;

    return h64;
}

// Hash an IPv4 address string (e.g. "192.168.1.1").
// Matches Go's xxhash.Sum64(net.ParseIP(ip)):
//   net.ParseIP returns 16-byte IPv4-mapped-IPv6 form:
//   {0,0,0,0,0,0,0,0,0,0,0xFF,0xFF,a,b,c,d}
static uint64_t xxhash64_ip(const char *ip_str) {
    uint8_t buf[16] = {0,0,0,0, 0,0,0,0, 0,0,0xFF,0xFF, 0,0,0,0};
    unsigned int a, b, c, d;
    if (sscanf(ip_str, "%u.%u.%u.%u", &a, &b, &c, &d) != 4) {
        return 0;
    }
    buf[12] = (uint8_t)a;
    buf[13] = (uint8_t)b;
    buf[14] = (uint8_t)c;
    buf[15] = (uint8_t)d;
    return xxhash64(buf, 16);
}

#endif // XXHASH64_H
