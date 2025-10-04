#pragma once
#include <stdint.h>
#include <stdbool.h>

typedef struct { double  v[3]; } vec3f64;
typedef struct { float   v[2]; } vec2f32;
typedef struct { int16_t v[3]; } vec3i16;
typedef struct { int8_t  v[2]; } vec2i8;
typedef uint8_t bool8;

void Bridge_ViewMoveEntityPos(uintptr_t h, int32_t id, const vec3i16* delta, bool8 onGround);
void Bridge_ViewMoveEntityPosAndRot(uintptr_t h, int32_t id, const vec3i16* delta, const vec2i8* rot, bool8 onGround);
void Bridge_ViewRemoveEntities(uintptr_t h, const int32_t* ids, int n);
void Bridge_ViewTeleportEntity(uintptr_t h, int32_t id, const vec3f64* pos, const vec2i8* rot, bool8 onGround);
void Bridge_SendPlayerPosition(uintptr_t h, const vec3f64* pos, const vec2f32* rot, int32_t* outTeleportID);
void Bridge_SendDisconnectJSON(uintptr_t h, const char* js, int n);
void Bridge_SendSetPlayerInventorySlot(uintptr_t h, int32_t slot, uintptr_t stackHandle);

typedef struct {
  void (*ViewMoveEntityPos)(uintptr_t, int32_t, const vec3i16*, bool8);
  void (*ViewMoveEntityPosAndRot)(uintptr_t, int32_t, const vec3i16*, const vec2i8*, bool8);
  void (*ViewRemoveEntities)(uintptr_t, const int32_t*, int);
  void (*ViewTeleportEntity)(uintptr_t, int32_t, const vec3f64*, const vec2i8*, bool8);
  void (*SendPlayerPosition)(uintptr_t, const vec3f64*, const vec2f32*, int32_t*);
  void (*SendDisconnectJSON)(uintptr_t, const char*, int);
  void (*SendSetPlayerInventorySlot)(uintptr_t, int32_t, uintptr_t);
} ClientCbs;

// 示例：把 Go 导出的符号填进回调表
static inline ClientCbs make_client_cbs(void) {
  ClientCbs cbs = {
    .ViewMoveEntityPos = &Bridge_ViewMoveEntityPos,
    .ViewMoveEntityPosAndRot = &Bridge_ViewMoveEntityPosAndRot,
    .ViewRemoveEntities = &Bridge_ViewRemoveEntities,
    .ViewTeleportEntity = &Bridge_ViewTeleportEntity,
    .SendPlayerPosition = &Bridge_SendPlayerPosition,
    .SendDisconnectJSON = &Bridge_SendDisconnectJSON,
    .SendSetPlayerInventorySlot = &Bridge_SendSetPlayerInventorySlot,
  };
  return cbs;
}