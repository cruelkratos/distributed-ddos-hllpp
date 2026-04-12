// hll.h — HLL++ (p=14) dense-only implementation for ESP32-C3.
// 6-bit packed registers, identical layout to Go's types/register/registers.go.
// Dense only — no sparse mode (12 KB trivial for 400 KB SRAM).
#ifndef HLL_H
#define HLL_H

#include <stdint.h>
#include <string.h>
#include <math.h>
#include "xxhash64.h"

#define HLL_P         14
#define HLL_M         (1 << HLL_P)                  // 16384
#define HLL_REG_BYTES ((HLL_M * 6 + 7) / 8)         // 12288

static uint8_t  hll_registers[HLL_REG_BYTES];
static uint32_t hll_window_id = 0;
static uint32_t hll_inserts   = 0;               // raw insert count this window

// --- 6-bit register access (identical to Go registers.go) ---

static inline uint8_t hll_get(int i) {
    int bitPos    = i * 6;
    int byteIndex = bitPos >> 3;
    int bitOffset = bitPos & 7;

    uint16_t cur = (uint16_t)hll_registers[byteIndex];
    if (byteIndex + 1 < HLL_REG_BYTES)
        cur |= (uint16_t)hll_registers[byteIndex + 1] << 8;
    return (uint8_t)((cur >> bitOffset) & 0x3F);
}

static inline void hll_set(int i, uint8_t v) {
    int bitPos    = i * 6;
    int byteIndex = bitPos >> 3;
    int bitOffset = bitPos & 7;

    uint16_t cur = (uint16_t)hll_registers[byteIndex];
    if (byteIndex + 1 < HLL_REG_BYTES)
        cur |= (uint16_t)hll_registers[byteIndex + 1] << 8;

    uint16_t mask = (uint16_t)0x3F << bitOffset;
    cur = (cur & ~mask) | ((uint16_t)(v & 0x3F) << bitOffset);

    hll_registers[byteIndex] = (uint8_t)(cur & 0xFF);
    if (byteIndex + 1 < HLL_REG_BYTES)
        hll_registers[byteIndex + 1] = (uint8_t)(cur >> 8);
}

// --- Rho: leading zeros in bottom `bitLength` bits of w, plus 1 ---
// Identical to Go general.Rho().

static inline int hll_rho(uint64_t w, int bitLength) {
    if (bitLength <= 0) return 1;
    if (bitLength < 64)
        w &= ((uint64_t)1 << bitLength) - 1;
    if (w == 0) return bitLength + 1;
    int lz = __builtin_clzll(w) - (64 - bitLength);
    return lz + 1;
}

// --- Insert an IPv4 address string into the sketch ---

static void hll_insert_ip(const char *ip_str) {
    uint64_t hash = xxhash64_ip(ip_str);
    int idx       = (int)(hash >> (64 - HLL_P));   // top 14 bits
    uint64_t w    = (hash << HLL_P) >> HLL_P;      // bottom 50 bits
    int r         = hll_rho(w, 64 - HLL_P);

    uint8_t cur = hll_get(idx);
    if ((uint8_t)r > cur)
        hll_set(idx, (uint8_t)r);
    hll_inserts++;
}

// --- Cardinality estimate (raw HLL + linear counting fallback) ---
// This is used on-device for display only; the aggregator applies full
// bias-corrected HLL++ after merging.

static uint64_t hll_count(void) {
    double alpha_m = 0.7213 / (1.0 + 1.079 / (double)HLL_M);
    double sum     = 0.0;
    int    zeros   = 0;

    for (int i = 0; i < HLL_M; i++) {
        uint8_t reg = hll_get(i);
        sum += pow(2.0, -(double)reg);
        if (reg == 0) zeros++;
    }

    double raw = alpha_m * (double)HLL_M * (double)HLL_M / sum;

    // Linear counting for small cardinalities.
    if (zeros > 0) {
        double lc = (double)HLL_M * log((double)HLL_M / (double)zeros);
        if (lc <= 72000.0)       // threshold for p=14
            return (uint64_t)(lc + 0.5);
    }
    return (uint64_t)(raw + 0.5);
}

// --- Reset registers for new window ---

static void hll_reset(void) {
    memset(hll_registers, 0, HLL_REG_BYTES);
    hll_window_id++;
    hll_inserts = 0;
}

#endif // HLL_H
