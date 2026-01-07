#ifndef __DEBUG_H__
#define __DEBUG_H__

#include <stdint.h>
#include <stdbool.h>
#include <stdarg.h>

// Debug levels
#define DBG_INIT        0x00000001
#define DBG_ERROR       0x00000002
#define DBG_CTRL        0x00000004
#define DBG_PACKET      0x00000008
#define DBG_SESSION     0x00000010
#define DBG_TIMER       0x00000020
#define DBG_TCP         0x00000040
#define DBG_PARSER      0x00000080
#define DBG_LOG         0x00000100
#define DBG_POLICY      0x00000200
#define DBG_DDOS        0x00000400
#define DBG_DEFAULT     (DBG_ERROR | DBG_CTRL)

#define DBG_MAC_FORMAT "%02x:%02x:%02x:%02x:%02x:%02x"
#define DBG_MAC_TUPLE(mac) \
    ((uint8_t *)&(mac))[0], ((uint8_t *)&(mac))[1], ((uint8_t *)&(mac))[2], \
    ((uint8_t *)&(mac))[3], ((uint8_t *)&(mac))[4], ((uint8_t *)&(mac))[5]
#define DBG_IPV4_FORMAT "%u.%u.%u.%u"
#define DBG_IPV4_TUPLE(ip) \
    ((uint8_t *)&(ip))[0], ((uint8_t *)&(ip))[1], \
    ((uint8_t *)&(ip))[2], ((uint8_t *)&(ip))[3]
#define DBG_IPV6_FORMAT "%x%x:%x%x:%x%x:%x%x:%x%x:%x%x:%x%x:%x%x"
#define DBG_IPV6_TUPLE(ip) \
    ((uint8_t *)&(ip))[0], ((uint8_t *)&(ip))[1], \
    ((uint8_t *)&(ip))[2], ((uint8_t *)&(ip))[3], \
    ((uint8_t *)&(ip))[4], ((uint8_t *)&(ip))[5], \
    ((uint8_t *)&(ip))[6], ((uint8_t *)&(ip))[7], \
    ((uint8_t *)&(ip))[8], ((uint8_t *)&(ip))[9], \
    ((uint8_t *)&(ip))[10], ((uint8_t *)&(ip))[11], \
    ((uint8_t *)&(ip))[12], ((uint8_t *)&(ip))[13], \
    ((uint8_t *)&(ip))[14], ((uint8_t *)&(ip))[15]

extern uint32_t g_debug_levels;

// Debug macros
#define DEBUG_LEVEL(level, fmt, args...) \
    do { \
        if (g_debug_levels & (level)) { \
            debug_func(true, fmt, ##args); \
        } \
    } while (0)

#define DEBUG_INIT(fmt, args...)    DEBUG_LEVEL(DBG_INIT, fmt, ##args)
#define DEBUG_ERROR(level, fmt, args...)   DEBUG_LEVEL((level) | DBG_ERROR, fmt, ##args)
#define DEBUG_CTRL(fmt, args...)    DEBUG_LEVEL(DBG_CTRL, fmt, ##args)
#define DEBUG_PACKET(fmt, args...)  DEBUG_LEVEL(DBG_PACKET, fmt, ##args)
#define DEBUG_SESSION(fmt, args...) DEBUG_LEVEL(DBG_SESSION, fmt, ##args)
#define DEBUG_TIMER(fmt, args...)   DEBUG_LEVEL(DBG_TIMER, fmt, ##args)
#define DEBUG_TCP(fmt, args...)     DEBUG_LEVEL(DBG_TCP, fmt, ##args)
#define DEBUG_PARSER(fmt, args...)  DEBUG_LEVEL(DBG_PARSER, fmt, ##args)
#ifndef DEBUG_LOG
#define DEBUG_LOG(fmt, args...)     DEBUG_LEVEL(DBG_LOG, fmt, ##args)
#endif
#define DEBUG_LOGGER(fmt, args...)  DEBUG_LEVEL(DBG_LOG, fmt, ##args)
#define DEBUG_POLICY(fmt, args...)  DEBUG_LEVEL(DBG_POLICY, fmt, ##args)

#define DEBUG_FUNC_ENTRY(level) \
    DEBUG_LEVEL(level, "Enter %s\n", __FUNCTION__)

// Debug functions
extern int debug_func(bool print_ts, const char *fmt, ...);
extern int debug_file(bool print_ts, const char *fmt, va_list args);
extern uint32_t debug_name2level(const char *name);
extern const char *debug_action_name(uint8_t action);

#endif // __DEBUG_H__
