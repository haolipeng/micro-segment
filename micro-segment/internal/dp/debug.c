#include <stdio.h>
#include <string.h>
#include <strings.h>
#include <stdarg.h>

#include "debug.h"
#include "defs.h"

uint32_t g_debug_levels = DBG_DEFAULT;

int debug_func(bool print_ts, const char *fmt, ...)
{
    va_list args;
    int ret;

    va_start(args, fmt);
    ret = debug_file(print_ts, fmt, args);
    va_end(args);

    return ret;
}

uint32_t debug_name2level(const char *name)
{
    if (strcasecmp(name, "all") == 0) {
        return 0xffffffff;
    } else if (strcasecmp(name, "init") == 0) {
        return DBG_INIT;
    } else if (strcasecmp(name, "error") == 0) {
        return DBG_ERROR;
    } else if (strcasecmp(name, "ctrl") == 0) {
        return DBG_CTRL;
    } else if (strcasecmp(name, "packet") == 0) {
        return DBG_PACKET;
    } else if (strcasecmp(name, "session") == 0) {
        return DBG_SESSION;
    } else if (strcasecmp(name, "timer") == 0) {
        return DBG_TIMER;
    } else if (strcasecmp(name, "tcp") == 0) {
        return DBG_TCP;
    } else if (strcasecmp(name, "parser") == 0) {
        return DBG_PARSER;
    } else if (strcasecmp(name, "log") == 0) {
        return DBG_LOG;
    } else if (strcasecmp(name, "policy") == 0) {
        return DBG_POLICY;
    }
    return 0;
}

const char *debug_action_name(uint8_t action)
{
    switch (action) {
    case DP_POLICY_ACTION_OPEN:
        return "open";
    case DP_POLICY_ACTION_ALLOW:
        return "allow";
    case DP_POLICY_ACTION_DENY:
        return "deny";
    case DP_POLICY_ACTION_VIOLATE:
        return "violate";
    case DP_POLICY_ACTION_CHECK_APP:
        return "check_app";
    default:
        return "unknown";
    }
}
