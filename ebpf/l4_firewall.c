//go:build ignore

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "GPL";

#define MAX_RULES 256

struct firewall_rule {
    __u16 port;
    __u8 protocol;  // IPPROTO_TCP or IPPROTO_UDP
    __u8 action;    // 0 = allow, 1 = block
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_RULES);
    __type(key, __u32);  // rule index
    __type(value, struct firewall_rule);
} firewall_rules SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u32);  // interface index
    __type(value, __u8);  // enabled flag
} enabled_interfaces SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32);
    __type(value, __u64);
} stats SEC(".maps");

enum {
    STAT_ALLOWED = 0,
    STAT_BLOCKED = 1,
    STAT_TCP_PACKETS = 2,
    STAT_UDP_PACKETS = 3,
};

static __always_inline int check_firewall_rules(__u16 port, __u8 protocol) {
    struct firewall_rule *rule;
    __u32 i;

    // Check rules (simplified linear search for small rule sets)
    for (i = 0; i < MAX_RULES; i++) {
        rule = bpf_map_lookup_elem(&firewall_rules, &i);
        if (!rule) {
            continue;
        }
        
        if (rule->port == port && rule->protocol == protocol) {
            return rule->action; // 0 = allow, 1 = block
        }
    }
    
    return 0; // Default allow
}

static __always_inline void update_stats(__u32 stat_key) {
    __u64 *count = bpf_map_lookup_elem(&stats, &stat_key);
    if (count) {
        __sync_fetch_and_add(count, 1);
    }
}

SEC("tc/ingress")
int l4_firewall_ingress(struct __sk_buff *skb) {
    __u32 ifindex = skb->ingress_ifindex;
    
    // Check if firewall is enabled for this interface
    __u8 *enabled = bpf_map_lookup_elem(&enabled_interfaces, &ifindex);
    if (!enabled || !*enabled) {
        return TC_ACT_OK;
    }
    
    void *data_end = (void *)(long)skb->data_end;
    void *data = (void *)(long)skb->data;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return TC_ACT_OK;
    }
    
    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return TC_ACT_OK;
    }
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return TC_ACT_OK;
    }
    
    __u16 dest_port = 0;
    __u8 protocol = ip->protocol;
    
    if (protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)(ip + 1);
        if ((void *)(tcp + 1) > data_end) {
            return TC_ACT_OK;
        }
        dest_port = bpf_ntohs(tcp->dest);
        update_stats(STAT_TCP_PACKETS);
    } else if (protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)(ip + 1);
        if ((void *)(udp + 1) > data_end) {
            return TC_ACT_OK;
        }
        dest_port = bpf_ntohs(udp->dest);
        update_stats(STAT_UDP_PACKETS);
    } else {
        return TC_ACT_OK; // Allow non-TCP/UDP traffic
    }
    
    int action = check_firewall_rules(dest_port, protocol);
    if (action == 1) { // Block
        update_stats(STAT_BLOCKED);
        return TC_ACT_SHOT;
    }
    
    update_stats(STAT_ALLOWED);
    return TC_ACT_OK;
}