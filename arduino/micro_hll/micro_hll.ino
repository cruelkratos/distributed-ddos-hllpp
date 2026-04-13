/*
 * micro_hll.ino — Dual-mode HyperLogLog on Arduino Uno (ATmega328P, 2KB SRAM)
 *
 * Mode A: Micro-HLL at precision p=4 (16 registers × 6 bits = 12 bytes)
 *         Proves the HLL algorithm runs on extreme-constrained hardware.
 *         Standard error: 1.04 / sqrt(16) ≈ 26% — terrible for production,
 *         but easily detects 10× DDoS spikes.
 *
 * Mode B: Lightweight sensor — forwards raw IPs to Raspberry Pi via serial.
 *         Pi runs full agent (p=14, 12KB sketch) with proper accuracy.
 *
 * Serial Protocol (115200 baud):
 *   Commands received:
 *     INSERT <ip>       — Insert IP into local micro-HLL AND forward to Pi
 *     ESTIMATE          — Return local HLL cardinality estimate
 *     RESET             — Reset the HLL sketch
 *     EXPORT            — Export raw register bytes (hex-encoded)
 *     MEMINFO           — Report memory usage
 *     MODE HLL          — Micro-HLL only (no forwarding)
 *     MODE SENSOR       — Sensor only (forward IPs, no local HLL)
 *     MODE DUAL         — Both (default)
 *     STATS             — Print insert count, estimate, and mode
 *
 *   Responses:
 *     OK                — Command succeeded
 *     EST:<value>       — Cardinality estimate
 *     MEM:<bytes>       — Memory used by registers
 *     REG:<hex>         — Hex-encoded register bytes
 *     FWD:<ip>          — Forwarded IP (sensor mode, sent to Pi)
 *     ERR:<message>     — Error
 *
 * Memory layout (p=4):
 *   Registers: 12 bytes (16 × 6-bit packed, same layout as Go implementation)
 *   Input buffer: 64 bytes
 *   Variables: ~60 bytes
 *   Stack: ~200 bytes
 *   Total: ~336 bytes of 2048 SRAM
 *
 * The register packing matches types/register/registers.go exactly:
 *   bitPos = i * 6, byteIndex = bitPos / 8, bitOffset = bitPos % 8
 *   Read:  (word >> bitOffset) & 0x3F
 *   Write: (word & ~mask) | (val << bitOffset)
 */

// ─── HLL Configuration ──────────────────────────────────────────────
#define HLL_P         4                   // precision: 4 bits for index
#define HLL_M         (1 << HLL_P)        // 16 registers
#define HLL_REG_BYTES ((HLL_M * 6 + 7) / 8)  // 12 bytes

// ─── Register storage ────────────────────────────────────────────────
static uint8_t registers[HLL_REG_BYTES];  // 12 bytes, zero-initialized

// ─── Operating mode ──────────────────────────────────────────────────
enum Mode { MODE_DUAL = 0, MODE_HLL = 1, MODE_SENSOR = 2 };
static Mode currentMode = MODE_DUAL;

// ─── Counters ────────────────────────────────────────────────────────
static uint32_t insertCount = 0;

// ─── Serial input buffer ────────────────────────────────────────────
#define BUF_SIZE 64
static char inputBuf[BUF_SIZE];
static uint8_t bufPos = 0;

// =====================================================================
// FNV-1a 32-bit hash — simple, fast on 8-bit AVR
// =====================================================================
#define FNV_OFFSET 2166136261UL
#define FNV_PRIME  16777619UL

static uint32_t fnv1a_hash(const char *data, uint8_t len) {
    uint32_t h = FNV_OFFSET;
    for (uint8_t i = 0; i < len; i++) {
        h ^= (uint8_t)data[i];
        h *= FNV_PRIME;
    }
    return h;
}

// =====================================================================
// Parse IPv4 string "A.B.C.D" into 4 bytes. Returns true on success.
// =====================================================================
static bool parseIPv4(const char *s, uint8_t out[4]) {
    uint8_t octet = 0;
    uint8_t idx = 0;
    bool hasDigit = false;

    for (const char *p = s; ; p++) {
        if (*p >= '0' && *p <= '9') {
            octet = octet * 10 + (*p - '0');
            hasDigit = true;
            if (octet > 255) return false;
        } else if (*p == '.' || *p == '\0') {
            if (!hasDigit || idx >= 4) return false;
            out[idx++] = octet;
            octet = 0;
            hasDigit = false;
            if (*p == '\0') break;
        } else {
            return false;
        }
    }
    return (idx == 4);
}

// =====================================================================
// Count leading zeros in a 32-bit value (CLZ)
// =====================================================================
static uint8_t clz32(uint32_t x) {
    if (x == 0) return 32;
    uint8_t n = 0;
    if ((x & 0xFFFF0000UL) == 0) { n += 16; x <<= 16; }
    if ((x & 0xFF000000UL) == 0) { n += 8;  x <<= 8;  }
    if ((x & 0xF0000000UL) == 0) { n += 4;  x <<= 4;  }
    if ((x & 0xC0000000UL) == 0) { n += 2;  x <<= 2;  }
    if ((x & 0x80000000UL) == 0) { n += 1; }
    return n;
}

// =====================================================================
// Rho function: position of first 1-bit in the lower (bitLength) bits.
// Matches general.Rho() in the Go codebase.
// =====================================================================
static uint8_t rho(uint32_t w, uint8_t bitLength) {
    if (bitLength == 0) return 1;
    // Keep only lower bitLength bits
    if (bitLength < 32) {
        w &= ((uint32_t)1 << bitLength) - 1;
    }
    if (w == 0) return bitLength + 1;
    // Leading zeros within the bitLength window
    uint8_t lz = clz32(w) - (32 - bitLength);
    return lz + 1;
}

// =====================================================================
// 6-bit packed register access (matches types/register/registers.go)
// =====================================================================
static uint8_t regGet(uint8_t i) {
    uint16_t bitPos = (uint16_t)i * 6;
    uint8_t byteIdx = bitPos / 8;
    uint8_t bitOff  = bitPos % 8;

    uint16_t cur = registers[byteIdx];
    if (byteIdx + 1 < HLL_REG_BYTES) {
        cur |= ((uint16_t)registers[byteIdx + 1]) << 8;
    }
    return (uint8_t)((cur >> bitOff) & 0x3F);
}

static void regSet(uint8_t i, uint8_t v) {
    uint16_t bitPos = (uint16_t)i * 6;
    uint8_t byteIdx = bitPos / 8;
    uint8_t bitOff  = bitPos % 8;

    uint16_t cur = registers[byteIdx];
    if (byteIdx + 1 < HLL_REG_BYTES) {
        cur |= ((uint16_t)registers[byteIdx + 1]) << 8;
    }

    uint16_t mask = (uint16_t)0x3F << bitOff;
    cur = (cur & ~mask) | ((uint16_t)(v & 0x3F) << bitOff);

    registers[byteIdx] = (uint8_t)(cur & 0xFF);
    if (byteIdx + 1 < HLL_REG_BYTES) {
        registers[byteIdx + 1] = (uint8_t)(cur >> 8);
    }
}

// =====================================================================
// HLL Insert: hash IP, extract index + rho, update register
// =====================================================================
static void hllInsert(const char *ip) {
    uint8_t ipBytes[4];
    if (!parseIPv4(ip, ipBytes)) return;

    uint32_t h = fnv1a_hash((const char *)ipBytes, 4);

    // Extract top HLL_P bits as index (same logic as Go: hash >> (bits - p))
    uint8_t idx = (uint8_t)(h >> (32 - HLL_P));

    // Remaining bits for rho
    uint32_t w = h << HLL_P;
    w = w >> HLL_P;  // clear the top p bits
    uint8_t r = rho(w, 32 - HLL_P);

    // Update register: max(current, rho)
    uint8_t cur = regGet(idx);
    if (r > cur) {
        regSet(idx, r);
    }
}

// =====================================================================
// HLL Estimate: standard HLL cardinality formula
// =====================================================================
static uint32_t hllEstimate(void) {
    // alpha_m for small m values
    float alpha_m;
    switch (HLL_M) {
        case 16:  alpha_m = 0.673;  break;
        case 32:  alpha_m = 0.697;  break;
        case 64:  alpha_m = 0.709;  break;
        default:  alpha_m = 0.7213 / (1.0 + 1.079 / (float)HLL_M); break;
    }

    // Harmonic mean of 2^(-register[i])
    float sum = 0.0;
    uint8_t zeros = 0;
    for (uint8_t i = 0; i < HLL_M; i++) {
        uint8_t val = regGet(i);
        if (val == 0) zeros++;
        // 2^(-val) using bit shift for integer part
        sum += 1.0 / (float)((uint32_t)1 << val);
    }

    float estimate = alpha_m * (float)HLL_M * (float)HLL_M / sum;

    // Small range correction: LinearCounting
    if (estimate <= 2.5 * HLL_M && zeros > 0) {
        estimate = (float)HLL_M * log((float)HLL_M / (float)zeros);
    }

    return (uint32_t)(estimate + 0.5);
}

// =====================================================================
// HLL Reset
// =====================================================================
static void hllReset(void) {
    memset(registers, 0, HLL_REG_BYTES);
    insertCount = 0;
}

// =====================================================================
// Command handlers
// =====================================================================

static void cmdInsert(const char *ip) {
    insertCount++;

    // Mode A: local micro-HLL
    if (currentMode == MODE_DUAL || currentMode == MODE_HLL) {
        hllInsert(ip);
    }

    // Mode B: forward to Pi via serial
    if (currentMode == MODE_DUAL || currentMode == MODE_SENSOR) {
        Serial.print(F("FWD:"));
        Serial.println(ip);
    }

    Serial.println(F("OK"));
}

static void cmdEstimate(void) {
    uint32_t est = hllEstimate();
    Serial.print(F("EST:"));
    Serial.println(est);
}

static void cmdReset(void) {
    hllReset();
    Serial.println(F("OK"));
}

static void cmdExport(void) {
    Serial.print(F("REG:"));
    for (uint8_t i = 0; i < HLL_REG_BYTES; i++) {
        if (registers[i] < 0x10) Serial.print('0');
        Serial.print(registers[i], HEX);
    }
    Serial.println();
}

static void cmdMeminfo(void) {
    Serial.print(F("MEM:"));
    Serial.println(HLL_REG_BYTES);
}

static void cmdStats(void) {
    Serial.print(F("INSERTS:"));
    Serial.println(insertCount);
    Serial.print(F("EST:"));
    Serial.println(hllEstimate());
    Serial.print(F("MODE:"));
    switch (currentMode) {
        case MODE_DUAL:   Serial.println(F("DUAL"));   break;
        case MODE_HLL:    Serial.println(F("HLL"));    break;
        case MODE_SENSOR: Serial.println(F("SENSOR")); break;
    }
    Serial.print(F("P:"));
    Serial.println(HLL_P);
    Serial.print(F("M:"));
    Serial.println(HLL_M);
    Serial.print(F("REG_BYTES:"));
    Serial.println(HLL_REG_BYTES);
}

static void cmdMode(const char *arg) {
    if (strcmp(arg, "DUAL") == 0) {
        currentMode = MODE_DUAL;
    } else if (strcmp(arg, "HLL") == 0) {
        currentMode = MODE_HLL;
    } else if (strcmp(arg, "SENSOR") == 0) {
        currentMode = MODE_SENSOR;
    } else {
        Serial.print(F("ERR:unknown mode "));
        Serial.println(arg);
        return;
    }
    Serial.println(F("OK"));
}

// =====================================================================
// Parse and dispatch a complete line
// =====================================================================
static void processCommand(char *line) {
    // Trim trailing whitespace
    uint8_t len = strlen(line);
    while (len > 0 && (line[len - 1] == '\r' || line[len - 1] == '\n' || line[len - 1] == ' ')) {
        line[--len] = '\0';
    }
    if (len == 0) return;

    // Find first space to split command and argument
    char *space = strchr(line, ' ');
    char *arg = NULL;
    if (space) {
        *space = '\0';
        arg = space + 1;
        // Trim leading spaces from arg
        while (*arg == ' ') arg++;
    }

    if (strcmp(line, "INSERT") == 0 && arg) {
        cmdInsert(arg);
    } else if (strcmp(line, "ESTIMATE") == 0) {
        cmdEstimate();
    } else if (strcmp(line, "RESET") == 0) {
        cmdReset();
    } else if (strcmp(line, "EXPORT") == 0) {
        cmdExport();
    } else if (strcmp(line, "MEMINFO") == 0) {
        cmdMeminfo();
    } else if (strcmp(line, "STATS") == 0) {
        cmdStats();
    } else if (strcmp(line, "MODE") == 0 && arg) {
        cmdMode(arg);
    } else {
        Serial.print(F("ERR:unknown command "));
        Serial.println(line);
    }
}

// =====================================================================
// Arduino setup & loop
// =====================================================================
void setup() {
    Serial.begin(115200);
    while (!Serial) { ; }  // Wait for serial port (Leonardo/Micro)

    hllReset();

    Serial.println(F("READY"));
    Serial.print(F("micro_hll p="));
    Serial.print(HLL_P);
    Serial.print(F(" m="));
    Serial.print(HLL_M);
    Serial.print(F(" reg_bytes="));
    Serial.println(HLL_REG_BYTES);
    Serial.println(F("Modes: DUAL (default), HLL, SENSOR"));
}

void loop() {
    while (Serial.available()) {
        char c = Serial.read();
        if (c == '\n' || c == '\r') {
            if (bufPos > 0) {
                inputBuf[bufPos] = '\0';
                processCommand(inputBuf);
                bufPos = 0;
            }
        } else if (bufPos < BUF_SIZE - 1) {
            inputBuf[bufPos++] = c;
        }
        // If buffer overflows, silently drop until newline
    }
}
