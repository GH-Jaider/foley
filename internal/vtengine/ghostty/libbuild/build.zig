// Extractor de libghostty-vt.a pineada.
//
// Este paquete no compila código propio: declara ghostty como dependencia
// por URL+hash (el pin) y instala su artefacto estático para el target
// pedido. Se ejecuta vía `make engine-lib` dentro del contenedor de build
// (zig 0.15.x; hoy roto en macOS 26, por eso el contenedor), produciendo
// zig-out/lib/libghostty-vt.a para linux nativo o cross a darwin.
const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    // simd=false: build autocontenido (sin el componente C++ ghostty_simd
    // aparte). Costo de perf en paths calientes de parsing — se re-evalúa
    // con benchmark propio si el throughput de VT lo pide.
    if (b.lazyDependency("ghostty", .{
        .target = target,
        .optimize = optimize,
        .simd = false,
    })) |dep| {
        b.installArtifact(dep.artifact("ghostty-vt-static"));
    }
}
