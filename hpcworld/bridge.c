#include "bridge.h"
#include <stdlib.h>
#include <string.h>
#include <time.h>

// Helper to fill biomes with plains (id=1 for example)
static void fill_biomes(int32_t *biomes, int size, int biome_id) {
    for (int i = 0; i < size; ++i) {
        biomes[i] = biome_id;
    }
}

// Helper to fill light arrays with random values
static void fill_light(int8_t *light, int size) {
    for (int i = 0; i < size; ++i) {
        light[i] = i % 16;
    }
}

// Helper to fill blocks_state with grass block (id=2 for example)
static void fill_grass_blocks(int32_t *blocks_state, int size, int block_id) {
    for (int i = 0; i < size; ++i) {
        blocks_state[i] = block_id;
    }
}

Chunk* load_chunk(int32_t x, int32_t z) {
    // Simulate chunk not exist, so generate empty chunk
    Chunk *c = (Chunk*)malloc(sizeof(Chunk));
    if (!c) return NULL;
    memset(c, 0, sizeof(Chunk));

    srand((unsigned int)(x ^ z ^ (int)time(NULL)));

    for (int s = 0; s < 24; ++s) {
        Section *sec = &c->sections[s];
        sec->blockcount = 0;
        fill_biomes(sec->biomes, 64, 1); // plains biome id
        fill_light(sec->sky_light, 2048);
        fill_light(sec->block_light, 2048);
        if (s == 10) {
            fill_grass_blocks(sec->blocks_state, 4096, 1); // grass block id
            for (int i = 0; i < 256; ++i) {
                sec->sky_light[i] = i % 16;
                sec->block_light[i] = i % 16;
            }
        }
    }
    return c;
}
