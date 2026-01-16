// Package quarkgl provides a minimal, predictable software 3D engine for Spark userland.
//
// QuarkGL is intended for visualization: meshes, simple scenes, and interactive views
// (orbit/zoom/pan). It is not a game engine and does not provide a GPU abstraction.
//
// Pipeline (fixed):
//
//	Scene → Transform → Projection → Clipping → Rasterization → Frame output.
//
// The renderer is software-only and draws into a caller-provided Target. The library
// does not require a full framebuffer and avoids allocations in the render hot path.
//
// Numeric backend:
//
// By default QuarkGL uses float32 math. A fixed-point backend can be selected at build
// time with the build tag `quarkgl_fixed` (currently minimal/stubbed; float is
// recommended).
package quarkgl
