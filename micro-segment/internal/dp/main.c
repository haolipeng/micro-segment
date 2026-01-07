// Simplified DP main program for micro-segment
// Removed: pcap support, shared memory, complex initialization

#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <signal.h>
#include <string.h>
#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>

#include "debug.h"
#include "apis.h"
#include "utils/helper.h"

// 时间函数
static inline uint32_t get_current_time(void) {
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    return ts.tv_sec;
}

// 虚拟端点初始化
static void init_dummy_ep(void *ep) {
    // 简化实现，仅用于编译
    (void)ep;
}
#include "utils/helper.h"
#include "utils/rcu_map.h"

// External functions
extern void *dp_timer_thr(void *args);
extern void *dp_data_thr(void *args);
extern void dp_ctrl_loop(void);
extern int dp_ctrl_send_json(json_t *root);
extern int dp_ctrl_send_binary(void *data, int len);

extern int dp_ctrl_threat_log(DPMsgThreatLog *log);
extern int dp_ctrl_traffic_log(DPMsgSession *log);
extern int dp_ctrl_connect_report(DPMsgSession *log, DPMonitorMetric *metric, int count_session, int count_violate);
extern void dp_ctrl_init_thread_data(void);
extern int dp_send_packet(io_ctx_t *context, uint8_t *pkt, int len);

// Global variables
__thread int THREAD_ID;
__thread char THREAD_NAME[32];
dp_mnt_shm_t *g_shm = NULL;


int g_running;
rcu_map_t g_ep_map;
struct cds_list_head g_subnet4_list; 
struct cds_list_head g_subnet6_list; 
struct timeval g_now;
int g_dp_threads = 0;
int g_stats_slot = 0;
pthread_mutex_t g_debug_lock;

io_callback_t g_callback;
io_config_t g_config;

// Signal handlers
static void dp_signal_dump_policy(int num)
{
    int thr_id;
    for (thr_id = 0; thr_id < g_dp_threads; thr_id++) {
        dp_data_wait_ctrl_req_thr(CTRL_REQ_DUMP_POLICY, thr_id);
    }
}

static void dp_signal_exit(int num)
{
    g_running = false;
}

// Debug functions
static inline int debug_ts(FILE *logfp)
{
    struct timeval now;
    struct tm *tm;

    if (g_now.tv_sec == 0) {
        time_t t = get_current_time();
        tm = localtime((const time_t *)&t);
    } else {
        now = g_now;
        tm = localtime(&now.tv_sec);
    }

    return fprintf(logfp, "%04d-%02d-%02dT%02d:%02d:%02d|DEBU|%s|",
                   tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday,
                   tm->tm_hour, tm->tm_min, tm->tm_sec, THREAD_NAME);
}

static int debug_stdout(bool print_ts, const char *fmt, va_list args)
{
    int len = 0;

    pthread_mutex_lock(&g_debug_lock);
    if (print_ts) {
        len = debug_ts(stdout);
    }
    len += vprintf(fmt, args);
    pthread_mutex_unlock(&g_debug_lock);

    return len;
}

int debug_file(bool print_ts, const char *fmt, va_list args)
{
    static FILE *logfp = NULL;
    const char *log_file = "/var/log/micro-segment/dp.log";

    if (logfp == NULL) {
        logfp = fopen(log_file, "a");
        if (logfp == NULL) {
            return debug_stdout(print_ts, fmt, args);
        }
    }

    int len = 0;
    pthread_mutex_lock(&g_debug_lock);
    if (print_ts) {
        len = debug_ts(logfp);
    }
    len += vfprintf(logfp, fmt, args);
    fflush(logfp);
    pthread_mutex_unlock(&g_debug_lock);

    return len;
}

// Main network processing
static int net_run(void)
{
    pthread_t timer_thr;
    pthread_t dp_thr[MAX_DP_THREADS];
    int i, timer_thr_id, thr_id[MAX_DP_THREADS];
    bool thr_create[MAX_DP_THREADS];

    DEBUG_FUNC_ENTRY(DBG_INIT);

    g_running = true;

    // Setup signal handlers
    signal(SIGTERM, dp_signal_exit);
    signal(SIGINT, dp_signal_exit);
    signal(SIGQUIT, dp_signal_exit);
    signal(SIGUSR1, dp_signal_dump_policy);

    // Initialize thread tracking
    for (i = 0; i < MAX_DP_THREADS; i++) {
        thr_create[i] = false;
    }

    // Calculate number of dp threads
    if (g_dp_threads == 0) {
        g_dp_threads = count_cpu();
    }
    if (g_dp_threads > MAX_DP_THREADS) {
        g_dp_threads = MAX_DP_THREADS;
    }

    printf("Starting DP with %d threads\n", g_dp_threads);

    dp_ctrl_init_thread_data();

    // Create timer thread
    pthread_create(&timer_thr, NULL, dp_timer_thr, &timer_thr_id);

    // Create data processing threads
    for (i = 0; i < g_dp_threads; i++) {
        thr_id[i] = i;
        thr_create[i] = true;
        pthread_create(&dp_thr[i], NULL, dp_data_thr, &thr_id[i]);
    }

    // Run control loop (blocks until exit)
    dp_ctrl_loop();

    // Wait for threads to finish
    pthread_join(timer_thr, NULL);
    for (i = 0; i < g_dp_threads; i++) {
        if (thr_create[i]) {
            pthread_join(dp_thr[i], NULL);
        }
    }

    return 0;
}

// EP map functions
static int dp_ep_match(struct cds_lfht_node *ht_node, const void *key)
{
    io_mac_t *ht_mac = STRUCT_OF(ht_node, io_mac_t, node);
    const uint8_t *mac = key;
    return memcmp(mac, &ht_mac->mac, sizeof(ht_mac->mac)) == 0 ? 1 : 0;
}

static uint32_t dp_ep_hash(const void *key)
{
    return sdbm_hash(key, ETH_ALEN);
}

// Help
static void help(const char *prog)
{
    printf("Micro-Segment DP (Data Plane)\n\n");
    printf("Usage: %s [options]\n\n", prog);
    printf("Options:\n");
    printf("  -h          Show this help\n");
    printf("  -d <level>  Debug level (none, all, error, ctrl, packet, session, policy)\n");
    printf("  -n <num>    Number of worker threads (default: auto)\n");
    printf("  -c <file>   Config file path\n");
    printf("\n");
}

// Main
int main(int argc, char *argv[])
{
    int arg = 0;
    struct rlimit core_limits;
    char *config_file = NULL;

    // Enable core dumps
    core_limits.rlim_cur = core_limits.rlim_max = RLIM_INFINITY;
    setrlimit(RLIMIT_CORE, &core_limits);

    // Parse arguments
    memset(&g_config, 0, sizeof(g_config));
    while ((arg = getopt(argc, argv, "hd:n:c:")) != -1) {
        switch (arg) {
        case 'd':
            if (strcasecmp(optarg, "none") == 0) {
                g_debug_levels = 0;
            } else if (optarg[0] == '-') {
                g_debug_levels &= ~debug_name2level(optarg + 1);
            } else {
                g_debug_levels |= debug_name2level(optarg);
            }
            break;
        case 'n':
            g_dp_threads = atoi(optarg);
            break;
        case 'c':
            config_file = optarg;
            break;
        case 'h':
        default:
            help(argv[0]);
            return arg == 'h' ? 0 : -1;
        }
    }

    setlinebuf(stdout);

    printf("Micro-Segment DP starting...\n");
    if (config_file) {
        printf("Config file: %s\n", config_file);
    }

    // Initialize
    pthread_mutex_init(&g_debug_lock, NULL);
    rcu_map_init(&g_ep_map, 1, offsetof(io_mac_t, node), dp_ep_match, dp_ep_hash);
    CDS_INIT_LIST_HEAD(&g_subnet4_list);
    CDS_INIT_LIST_HEAD(&g_subnet6_list);

    // Allocate shared memory structure (standalone mode)
    g_shm = calloc(1, sizeof(dp_mnt_shm_t));
    if (g_shm == NULL) {
        printf("ERROR: Failed to allocate shared memory structure\n");
        return -1;
    }

    init_dummy_ep(&g_config.dummy_ep);
    g_config.dummy_mac.ep = &g_config.dummy_ep;

    // Setup callbacks
    g_callback.debug = debug_stdout;
    g_callback.send_packet = dp_send_packet;
    g_callback.send_ctrl_json = dp_ctrl_send_json;
    g_callback.send_ctrl_binary = dp_ctrl_send_binary;
    // 简化版本暂时注释掉这些回调
    // g_callback.threat_log = dp_ctrl_threat_log;
    // g_callback.traffic_log = dp_ctrl_traffic_log;
    // g_callback.connect_report = dp_ctrl_connect_report;

    dpi_setup(&g_callback, &g_config);

    // Run
    int ret = net_run();

    printf("DP exiting...\n");
    return ret;
}
