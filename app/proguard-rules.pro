# Keep readable stack traces; the native bridge relies on exact class names.
-dontobfuscate

# gomobile / Go <-> Java bridge. These classes are referenced from native code
# (libgojni.so) by name, so they must not be shrunk or renamed.
-keep class go.** { *; }
-keep class io.github.revocx35.zapret.** { *; }

# Our callbacks are invoked from native code.
-keep class io.github.revocx35.zapretmobile.engine.** { *; }
