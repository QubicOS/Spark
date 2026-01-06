//go:build !tinygo

#include <stdint.h>

#include "lfs.h"

extern int go_lfs_read(void *ctx, lfs_block_t block, lfs_off_t off, void *buffer, lfs_size_t size);
extern int go_lfs_prog(void *ctx, lfs_block_t block, lfs_off_t off, void *buffer, lfs_size_t size);
extern int go_lfs_erase(void *ctx, lfs_block_t block);
extern int go_lfs_sync(void *ctx);

static int spark_lfs_read(const struct lfs_config *c, lfs_block_t block, lfs_off_t off, void *buffer, lfs_size_t size) {
    return go_lfs_read(c->context, block, off, buffer, size);
}

static int spark_lfs_prog(const struct lfs_config *c, lfs_block_t block, lfs_off_t off, const void *buffer, lfs_size_t size) {
    return go_lfs_prog(c->context, block, off, (void*)buffer, size);
}

static int spark_lfs_erase(const struct lfs_config *c, lfs_block_t block) {
    return go_lfs_erase(c->context, block);
}

static int spark_lfs_sync(const struct lfs_config *c) {
    return go_lfs_sync(c->context);
}

void spark_lfs_config_init(struct lfs_config *cfg) {
    cfg->read = spark_lfs_read;
    cfg->prog = spark_lfs_prog;
    cfg->erase = spark_lfs_erase;
    cfg->sync = spark_lfs_sync;
}

