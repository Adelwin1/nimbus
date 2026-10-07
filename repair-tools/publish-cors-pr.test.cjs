const test = require("node:test");
const assert = require("node:assert/strict");
const {expectedRepair} = require("./publish-cors-pr.cjs");
test("repairs an active PUT route and preserves the rest of the source", () => {
 const source = 'package middleware\nconst methods = "GET, POST, PATCH, DELETE, OPTIONS"\n';
 assert.equal(expectedRepair(source, ' router.Put("/rules", handler)\n'), source.replace('POST, PATCH', 'POST, PUT, PATCH'));
});
test("ignores commented routes and already fixed sources", () => {
 const source = 'const methods = "GET, POST, PATCH, DELETE, OPTIONS"';
 assert.throws(() => expectedRepair(source, '// router.Put("/rules", handler)'), /No supported/);
 assert.throws(() => expectedRepair(source.replace('POST, PATCH', 'POST, PUT, PATCH'), ' router.Put("/rules", handler)'), /No supported/);
});
test("rejects ambiguous method declarations", () => {
 assert.throws(() => expectedRepair('"GET, POST, PATCH"\n"GET, POST, DELETE"', 'r.Put("/x", h)'), /Ambiguous/);
});
