# Rust — Phase 1 Course (Beginner Edition)

> Memory Safety Without a Garbage Collector.
> Three lessons. Teaching mode is gentle and explains every term. Drills are harsh.
> Assumes ~1 year of programming experience in any language. **Zero** Rust knowledge required.

---

## Before You Start: What Rust Actually Is

Rust is a **systems programming language**, like C and C++. "Systems programming" means writing code that runs close to the hardware: operating systems, browsers, databases, game engines, embedded firmware, network protocol implementations. It also means writing application code that needs to be very fast and very predictable.

Rust is an alternative to C and C++ that aims to be just as fast — same machine code performance, no runtime overhead — but without an entire category of bugs that have plagued those languages for fifty years. Specifically, the bugs Rust prevents are **memory-safety bugs**: use-after-free, double-free, buffer overflow, data races, null-pointer dereferences. In C and C++, these bugs are common and catastrophic. According to public statements from Microsoft and Google, roughly **70% of all security vulnerabilities** in their major products (Windows, Chrome) are memory-safety bugs in C/C++ code. They also cause non-security bugs that crash programs, corrupt user data, and produce mysterious behavior at 3 AM in production.

Languages like Java, Go, Python, JavaScript, and C# avoid most of these bugs by using a **garbage collector** — a runtime component that periodically scans your program's memory and frees the parts you're no longer using. The cost: performance overhead, periodic GC pauses, and a runtime requirement that prevents you from running these languages in environments without a runtime (operating-system kernels, microcontrollers, browser engines, and so on).

Rust prevents these bugs **at compile time**, with **no runtime overhead**. The compiler does the work. The cost: you, the programmer, have to learn a new way of thinking about memory, called the **ownership model**. This is what makes Rust hard to learn. It's also what makes Rust uniquely Rust.

If you've used Go, Python, JavaScript, or Java, you've probably never had to think about memory. The compiler and runtime did it for you. In Rust, you think about memory all the time. At first this is painful — every Rust learner goes through a "fighting the borrow checker" phase that lasts a few weeks. The compiler rejects code that looks obviously correct, and you have to puzzle out why. After enough weeks, it becomes second nature, and you start seeing those same memory patterns in other languages — you become a better programmer in *every* language because Rust forced you to articulate things that other languages let you ignore.

This 101 is structured around the three things that make Rust *Rust*:

1. **Ownership, borrowing, and lifetimes** — the compile-time memory model.
2. **The type system** — enums, traits, generics, error handling.
3. **Concurrency and shared state** — sending data between threads, smart pointers, async.

Don't try to absorb all three lessons in one sitting. Plan on roughly 6–10 hours per lesson, including drills. Your first week will feel like running through wet sand. By week three, the patterns will start to click. By month two, you'll write Rust without consulting the borrow checker rules consciously.

Today's stable Rust is **1.95** with the **2024 edition**. Editions are Rust's mechanism for opt-in language changes that might be backward-incompatible — your `Cargo.toml` declares which edition your crate uses, and the compiler matches that. The default for new projects is 2024 and that's what this course uses.

---

## A Small Glossary You'll See A Lot

I'll explain these properly as they come up, but bookmark this for quick reference.

* **Compile time** — when your source code is being turned into a binary by `rustc`, the Rust compiler. Rust does aggressive checks here.
* **Runtime** — when your compiled program is actually executing on a CPU.
* **Stack** — fast memory whose size and layout are known at compile time. Allocations and frees are just adjustments of a pointer.
* **Heap** — memory whose size is decided at runtime, requested from the OS in pieces. Slower to allocate and free.
* **Pointer / reference** — a value that says "the actual data lives over there at this memory address." A pointer is 8 bytes on a 64-bit machine.
* **Ownership** — Rust's idea that every piece of heap data has exactly one owner at any moment, and when the owner goes out of scope, the data is freed.
* **Move** — when you pass or assign an owned value, ownership transfers and the old binding can no longer be used.
* **Borrow** — a temporary reference to data you don't own. You can read or write through the reference, but you can't free the data, and the reference must not outlive the data.
* **Lifetime** — how long a reference is valid. Tracked by the compiler. Sometimes you have to write lifetime annotations like `'a`.
* **The borrow checker** — the part of `rustc` that enforces ownership and lifetime rules. The thing you'll be fighting at first.
* **Trait** — a set of methods a type promises to implement. Roughly Rust's equivalent of a Java interface or a Go interface, but more powerful.
* **Generic** — a function or type that works for many types, parameterised with `<T>`.
* **Crate** — a Rust library or binary package. The unit of compilation and distribution.
* **Cargo** — Rust's build tool and package manager. Like `npm` + `webpack`, or `go mod` + `go build`, all in one.
* **rustc** — the Rust compiler itself. You usually invoke it via `cargo build`.
* **`Option<T>`** — a built-in type that means "either a `T` or nothing." Rust's substitute for null.
* **`Result<T, E>`** — a built-in type that means "either an OK `T` or an error `E`." Rust's substitute for exceptions.
* **`unsafe`** — a keyword that lets you bypass some of the borrow checker's rules. For when you really need it. Rare in application code.

---

# Lesson 1: Ownership, Borrowing, and Lifetimes

## 1.1 Why This Lesson Exists

In April 2014, a security researcher discovered a bug in OpenSSL — the library that almost every HTTPS server in the world used at the time. The bug was in a feature called "heartbeat." When a client sent a heartbeat message, the server was supposed to echo back a few bytes the client had sent. The C code did roughly:

```c
memcpy(response, request->payload, request->length);
```

The catch: the actual bytes the client sent might be only 16 bytes long, but the client could *claim* the length was 65,535. The server would dutifully copy 65,535 bytes from its memory into the response — the first 16 from the client's request, and **65,519 bytes of whatever happened to be sitting in the server's memory afterward**. Things like passwords, private cryptographic keys, decrypted messages from other users.

This bug is called **Heartbleed**. It affected something like two-thirds of all servers on the internet. Every major company spent a frantic week patching servers and rotating keys. The total cost was estimated in the hundreds of millions of dollars.

Heartbleed was a **buffer overread** — the program read past the end of an array. In any garbage-collected language, this is impossible because every array access is bounds-checked at runtime. In Rust, it's also impossible, but for a slightly different and more interesting reason: Rust knows at compile time how big the array is and what is and isn't a valid index, and it inserts bounds checks where required. Either way, that one bug doesn't happen in Rust.

There are roughly six categories of memory bugs that destroy C/C++ programs:

1. **Use-after-free** — you free memory, then read or write through a pointer to it.
2. **Double-free** — you free the same memory twice; the allocator's bookkeeping gets corrupted.
3. **Buffer overflow / overread** — you read or write past the end of an array.
4. **Null-pointer dereference** — you read or write through a pointer that's `NULL`.
5. **Data races** — two threads access the same data at the same time, at least one is writing, and it's not synchronised.
6. **Memory leaks** — you allocate and never free; memory grows until the program is killed.

Garbage collected languages prevent #1, #2, #6 at the cost of GC pauses. They prevent #3 at the cost of bounds-check overhead. They typically don't prevent #5 (Java, Go, C# all have data races). They sometimes don't prevent #4 (Java's `NullPointerException`).

Rust prevents 1–5 at compile time, with no runtime overhead beyond bounds checks (which the optimiser often eliminates). It does not strictly prevent #6 — a memory leak in Rust is considered safe, just incorrect — but the patterns Rust pushes you toward make leaks unusual.

This lesson teaches you the system that makes that possible: **ownership**, **borrowing**, and **lifetimes**. They are the most foreign things about Rust if you come from a managed language, and they are responsible for almost every confusing compiler error you'll encounter in your first month.

## 1.2 How Memory Actually Works

Before we can talk about who owns what, we need to talk about where memory lives. A running program has access to two main pools of memory: the **stack** and the **heap**. They behave very differently. Most programmers in managed languages have a vague mental model of "memory" as one big undifferentiated pool, and that vagueness is exactly what we're going to fix.

### The stack

Imagine a stack of plates. You can put a plate on top, or take one off the top. You can't pull one out from the middle without dumping the plates on top of it.

Function calls work like this. When `main` calls `compute`, the language pushes a "stack frame" for `compute` on top of `main`'s stack frame. The frame holds `compute`'s local variables, arguments, and a return address. When `compute` returns, its frame is popped off and the memory is instantly available for the next function call.

```
  ┌──────────────┐   <- stack pointer
  │ compute's    │
  │ frame        │
  ├──────────────┤
  │ main's       │
  │ frame        │
  ├──────────────┤
  │ ...          │
  └──────────────┘
```

Stack allocation is essentially free. The CPU has a special register called the **stack pointer**, and "allocating" 32 bytes on the stack just means subtracting 32 from that register. "Freeing" is adding 32 back. No bookkeeping, no metadata.

The constraint: every variable on the stack must have a size known at compile time. The compiler has to know how much to subtract from the stack pointer when a function starts. So an `i32` (4 bytes) lives on the stack. A struct with two `i32`s (8 bytes) lives on the stack. A `[i32; 100]` (an array of exactly 100 ints, 400 bytes) lives on the stack. But a vector whose length we'll decide at runtime *cannot* live entirely on the stack — its size isn't known at compile time.

### The heap

The **heap** is a large pool of memory the operating system gives your program. When you need a chunk of unknown-at-compile-time size, you ask a piece of code called the **allocator** for some, and it gives you back a pointer to a piece of memory big enough for what you need.

```rust
let v: Vec<i32> = vec![1, 2, 3, 4, 5];
//      ^^^^^^^
//      A Vec is essentially three things on the stack:
//        - a pointer to a heap buffer
//        - a length (5)
//        - a capacity (>= 5)
//      The actual integers 1, 2, 3, 4, 5 live in a heap buffer.
```

Visually:

```
  Stack frame                   Heap
  ┌─────────────┐               ┌────────────────────┐
  │ v.ptr ──────┼──────────────▶│ 1 │ 2 │ 3 │ 4 │ 5 │
  │ v.len = 5   │               └────────────────────┘
  │ v.cap = 5   │
  └─────────────┘
```

Heap allocations have two properties stack allocations don't:

* They survive past the function that created them — you can return the heap pointer.
* They have to be **freed** when nobody needs them anymore, or they leak.

In C, you free heap memory by calling `free(pointer)`. The programmer is responsible for getting this exactly right. Forget to call it: leak. Call it twice: double-free. Call it and then keep using the pointer: use-after-free. These three bugs are the source of most C memory disasters.

In Java/Go/Python, you never call free; the garbage collector finds unreferenced memory and frees it for you. Reliable, but you pay the GC cost.

Rust does it a third way: the compiler figures out *exactly* when each heap allocation should be freed, and emits the free call automatically. This is possible because Rust's ownership rules let the compiler always answer the question "when is this no longer needed?"

## 1.3 The Garbage Collector and Its Costs

It's worth understanding what GC actually does, so you appreciate what Rust avoids. Skip this section if you already know.

A garbage collector is a piece of code that runs periodically and finds heap memory that nothing in your program can reach anymore. The classic algorithm is **mark-and-sweep**:

1. **Mark.** Start from the **roots** — global variables and the live local variables on every thread's stack. Recursively follow every pointer. Mark every heap object you can reach.
2. **Sweep.** Walk the heap. Anything not marked is garbage. Reclaim the memory.

This works perfectly. It also has costs:

* **CPU overhead.** Marking and sweeping takes time. Even concurrent collectors that run alongside your program use 5–20% of your CPU.
* **Pauses.** Most GCs have brief moments where they need to stop the program to maintain consistency. Modern Java and Go GCs keep these under 1ms most of the time, but in latency-sensitive applications (matching engines, game loops, audio processing) even 1ms is too long.
* **Memory overhead.** GCs typically need 2–3x as much memory as the program actually uses, because they need headroom to be efficient.
* **Indirection.** Most GC'd languages box small objects (an `Integer` in Java is a heap allocation, not 4 bytes on the stack). This kills cache locality.
* **Runtime requirement.** You can't run Java or Go on a microcontroller with 16KB of RAM. There's no room for the runtime.

For most software, you should pay these costs gladly. Manual memory management is hard. Rust's pitch is that it gives you manual memory management's performance with the GC's safety. The catch is the learning curve.

## 1.4 The Manual-Memory Disaster, in Detail

Let me show you what goes wrong without a GC and without Rust's discipline. This is C:

```c
int* make_array() {
    int x[5] = {1, 2, 3, 4, 5};
    return x;   // BUG: returns pointer to local array
}
// When make_array returns, x's stack frame is gone. The pointer
// now points at memory that's been reused by some other function's frame.
// Reading through it gives you whatever junk is there. Writing through
// it corrupts that other function's local variables. The C compiler
// will warn but won't stop you.

void use_after_free() {
    int* p = malloc(sizeof(int));
    *p = 42;
    free(p);
    *p = 43;   // BUG: write to freed memory. The allocator may have
               // already given that memory to someone else.
}

void double_free() {
    int* p = malloc(sizeof(int));
    free(p);
    free(p);   // BUG: corrupts the allocator's internal data structures.
               // May crash now, may crash an hour from now.
}

void leak() {
    int* p = malloc(sizeof(int));
    // BUG: forgot to free. Repeated calls leak memory forever.
}

void overflow() {
    int x[5];
    x[10] = 0; // BUG: writes past the array, into whatever variable
               // happens to be next on the stack. The C compiler does
               // not check; the CPU does not check.
}
```

Each of these bugs is silent at the moment of the bug. The program doesn't crash here. It crashes some indefinite time later, usually in a different function, with a stack trace that has nothing to do with the actual problem. Hunting these bugs is a major reason senior C programmers exist.

Rust eliminates all five of these patterns. The compiler refuses to compile any of them. Let's see how.

## 1.5 Ownership: Three Rules

Rust's whole memory model rests on three rules.

> **Rule 1.** Every value has a single owner.
>
> **Rule 2.** When the owner goes out of scope, the value is dropped (freed).
>
> **Rule 3.** Ownership can be transferred, but never duplicated, except for cheap types that opt in.

Memorise this. We'll spend the rest of the lesson unpacking it.

### Rule 1 in code

```rust
fn main() {
    let s = String::from("hello");
    //  ^
    //  s is the owner of the heap-allocated string buffer "hello".
    //  No other binding refers to it.
}
//   ^
//   At the closing brace, s goes out of scope. Rust calls s.drop()
//   automatically, which frees the heap buffer. No leak.
```

`String::from("hello")` allocates a heap buffer holding `h`, `e`, `l`, `l`, `o` (5 bytes), plus the `String` struct on the stack (3 fields: pointer, length, capacity = 24 bytes). When `main` ends, `s` goes out of scope. Rust automatically generates a call to `s.drop()`, which frees the heap buffer. You don't write the free. You can't forget the free. You can't double-free.

### Rule 2: drop

When a variable goes out of scope, Rust calls a method called `drop` on it. For types that own heap memory, `drop` releases that memory. For types that own files, `drop` closes the file. For mutexes, `drop` releases the lock. The trait that defines `drop` is called `Drop`, and it's how Rust does what C++ calls "RAII" — Resource Acquisition Is Initialization.

```rust
struct LoggedString {
    s: String,
}

impl Drop for LoggedString {
    fn drop(&mut self) {
        println!("dropping {}", self.s);
    }
}

fn main() {
    let _a = LoggedString { s: String::from("first") };
    let _b = LoggedString { s: String::from("second") };
}
// Output:
//   dropping second
//   dropping first
//
// Reverse order of declaration. Resources are freed in LIFO order
// at the end of the scope, like popping the stack.
```

You almost never write `Drop` by hand for application code. The standard library implements it for you on `String`, `Vec`, `Box`, `File`, `Mutex`, etc. You just trust that "when the owner goes out of scope, things get cleaned up correctly." This is the magic.

### Rule 3: move semantics

Here's where it gets interesting. Watch carefully:

```rust
fn main() {
    let s1 = String::from("hello");
    let s2 = s1;          // <-- ownership transfers from s1 to s2.

    println!("{}", s2);   // OK, s2 is the owner now.
    println!("{}", s1);   // COMPILE ERROR: borrow of moved value: s1.
}
```

The compiler error reads:

```
error[E0382]: borrow of moved value: `s1`
 --> src/main.rs:5:20
  |
2 |     let s1 = String::from("hello");
  |         -- move occurs because `s1` has type `String`,
  |            which does not implement the `Copy` trait
3 |     let s2 = s1;
  |              -- value moved here
4 |     println!("{}", s2);
5 |     println!("{}", s1);
  |                    ^^ value borrowed here after move
```

Read that error carefully. It's saying:

> You moved `s1` into `s2`. After a move, the old binding is invalid. You can't use `s1` anymore.

What's actually happening at the machine level: nothing. The bytes don't move. `let s2 = s1;` is just a `memcpy` of the 24-byte `String` header (pointer, length, capacity) from one stack slot to another. Both stack slots now contain identical bytes pointing at the same heap buffer. The compiler then declares `s1` "moved" and refuses to let you use it.

Why does this matter? Because if the compiler let you use both `s1` and `s2`, when each goes out of scope it would call `drop`, and you'd have a **double-free** of the heap buffer. The compiler prevents this by enforcing the rule "after a move, the old binding is dead."

### The same rule applies to function calls

```rust
fn takes_ownership(s: String) {
    println!("{}", s);
}   //  ^ s goes out of scope, drop is called, heap buffer freed.

fn main() {
    let s1 = String::from("hello");
    takes_ownership(s1);   // <-- s1 moved into the function.
    println!("{}", s1);    // COMPILE ERROR: s1 was moved.
}
```

Passing `s1` to `takes_ownership` is a move. The function is now the owner. When the function ends, the value is dropped. The caller can't use `s1` anymore.

This is shocking the first time you see it. In Java or Go, passing a string to a function leaves the caller free to keep using it. In Rust, you have to choose: do I want to give this away, or do I want the caller to keep it?

If you want the caller to keep it, you don't move. You **borrow** (next section).

### What about primitives?

```rust
fn main() {
    let x: i32 = 5;
    let y = x;
    println!("{} {}", x, y);   // Works fine. No error.
}
```

Why does this work? Because `i32` implements a built-in trait called `Copy`. Types that are `Copy` are duplicated bit-by-bit on assignment instead of being moved. Both `x` and `y` are valid afterwards. They each hold an independent 4-byte integer.

`Copy` is opt-in and only safe for types that have no resources to clean up. The standard `Copy` types are: all the integer types, `f32`/`f64`, `bool`, `char`, and tuples or arrays *of `Copy` types*. Custom structs are `Copy` only if you derive it (`#[derive(Copy, Clone)]`) and all their fields are `Copy`.

`String`, `Vec`, `Box`, etc., all own heap memory. They are emphatically not `Copy`. Bit-copying a `String` would give you two pointers to the same heap buffer, which is exactly the double-free scenario we're trying to avoid.

### When you need a real duplicate: clone

If you want a deep copy of a non-`Copy` type, you call `.clone()`:

```rust
fn main() {
    let s1 = String::from("hello");
    let s2 = s1.clone();   // Allocates a new heap buffer, copies the bytes.
    println!("{} {}", s1, s2);   // Both work.
}
```

Cloning is explicit and visible. You can see in the source code "this is doing a heap allocation and a memcpy." If you do it in a tight loop, you can spot the cost. Compare to languages like Python where `s2 = s1` *might* be a copy or a reference depending on the type, and you have to remember which.

The Rust style: prefer borrowing (next section). Clone only when you must.

## 1.6 Borrowing: References

You usually don't want to give ownership away. You want to lend something to a function temporarily, get it back, and keep using it. That's a **borrow**, written `&`.

```rust
fn print_length(s: &String) {
    println!("length is {}", s.len());
}   //  s goes out of scope. But s is just a reference; the underlying
    //  String is not dropped. The owner still has it.

fn main() {
    let s1 = String::from("hello");
    print_length(&s1);     // Pass a reference. No move.
    println!("{}", s1);    // Still works. We never gave up ownership.
}
```

Read carefully. The function signature `s: &String` says "give me a reference to a String, not a String." The call site `print_length(&s1)` says "here's a reference to my String." When the function ends, the reference goes out of scope, but the underlying `String` does not — the caller still owns it.

A reference is, mechanically, a pointer. It's 8 bytes on a 64-bit machine. It's exactly as cheap as a C pointer. Rust just adds compile-time rules about how you can use it.

### Two flavours of reference

Rust has two kinds of references:

* `&T` — **shared reference**, also called immutable borrow. You can read through it. You cannot write through it. You can have many of these at once.
* `&mut T` — **exclusive reference**, also called mutable borrow. You can read and write through it. While it exists, no other reference (shared or exclusive) to the same data is allowed.

```rust
fn modify(s: &mut String) {
    s.push_str(", world");   // Mutating through the &mut reference.
}

fn main() {
    let mut s = String::from("hello");
    //  ^^^
    //  Note: variable bindings are immutable by default. We have to
    //  declare s as mut to be allowed to take a &mut reference.

    modify(&mut s);
    println!("{}", s);   // "hello, world"
}
```

### The Aliasing-XOR-Mutability rule

This is *the* rule. Memorise it. Almost every borrow-checker error you'll see comes back to this.

> **At any moment, for any piece of data, you can have either:
>   • any number of shared references (`&T`), or
>   • exactly one exclusive reference (`&mut T`).
> Never both at the same time.**

Visually:

```
  Allowed:        &T    &T    &T    &T       (many readers)
  Allowed:        &mut T                     (one writer, no readers)
  FORBIDDEN:      &T    &mut T               (any reader + a writer)
  FORBIDDEN:      &mut T    &mut T           (two writers)
```

Why this rule? Two reasons.

**Reason 1: it eliminates data races at compile time.** A data race needs two threads accessing the same data with at least one writing. If at most one thread can have a writer (`&mut T`) and any thread with `&T` can't be writing, you literally cannot construct a data race in safe Rust. Lesson 3 will explore this.

**Reason 2: it eliminates iterator invalidation, dangling pointers from realloc, and a host of subtler bugs even in single-threaded code.** Consider:

```rust
fn main() {
    let mut v = vec![1, 2, 3];
    let first = &v[0];        // shared borrow of the first element.
    v.push(4);                // ERROR: cannot borrow v as mutable
                              // because it's already borrowed as immutable.
    println!("{}", first);
}
```

Why is this an error? Because `Vec::push` might trigger a re-allocation. If the vector's capacity is full, it allocates a new, larger heap buffer, copies the old elements over, and frees the old buffer. After that, `first` would be a dangling pointer into freed memory. Reading from it would be a use-after-free.

C++ has exactly this bug. You write a loop iterating over a vector, you call `push_back` inside the loop, and your iterator is now invalid. Crashes ensue. C++ "documents" this — the standard says iterators are invalidated by `push_back`. You're supposed to remember.

Rust prevents it at compile time. The shared borrow `first` and the mutable call `v.push(4)` (which conceptually takes `&mut v`) cannot coexist. The compiler refuses.

### A worked example

Let me walk through a borrow-checker conversation in slow motion.

```rust
fn main() {
    let mut s = String::from("hello");

    let r1 = &s;        // (a) shared borrow of s
    let r2 = &s;        // (b) another shared borrow of s
    println!("{} {}", r1, r2);   // (c) last use of r1 and r2

    let r3 = &mut s;    // (d) exclusive borrow of s
    r3.push_str(", world");
    println!("{}", r3);
}
```

The compiler tracks the lifetime of each borrow. After (c), `r1` and `r2` are never used again, so their borrows end. By (d), no borrows are outstanding, so taking a `&mut` is legal. This is called **non-lexical lifetimes** (NLL): a borrow's lifetime ends at its last use, not at the end of the lexical scope. NLL was added in 2018 and made the borrow checker dramatically more pleasant.

Now contrast:

```rust
fn main() {
    let mut s = String::from("hello");

    let r1 = &s;
    let r3 = &mut s;    // ERROR: cannot borrow s as mutable
                        // because it's also borrowed as immutable
    println!("{}", r1);
    r3.push_str(", world");
}
```

Here, `r1` is used after we try to take `r3`, so the shared borrow is still live when we attempt the exclusive borrow. Forbidden.

### The dangling-reference rule

You also can't return a reference to a local variable.

```rust
fn dangle() -> &String {       // ERROR
    let s = String::from("hello");
    &s
}   // s is dropped here. The reference would point to freed memory.
```

The C compiler will let you do this with a warning. Rust refuses to compile it. The error mentions a "missing lifetime specifier," which leads us into the next section.

## 1.7 Lifetimes

A **lifetime** is the duration over which a reference is valid. Every reference has one. Most of the time the compiler figures it out and you never write it. Sometimes you have to write it explicitly with a `'a` syntax.

Why we need them: consider this function.

```rust
fn longest(x: &str, y: &str) -> &str {
    if x.len() > y.len() { x } else { y }
}
```

The compiler rejects this. Why? Because the returned reference might point to whatever `x` points to, or whatever `y` points to. The caller needs to know how long the returned reference is valid for. Specifically, the returned reference is valid for the *shorter* of the two input lifetimes — once either input goes away, the output might be dangling.

We have to tell the compiler this:

```rust
fn longest<'a>(x: &'a str, y: &'a str) -> &'a str {
    if x.len() > y.len() { x } else { y }
}
```

`'a` is a lifetime parameter. Read the signature as: "for some lifetime `'a`, take two references that both live at least `'a`, and return a reference that also lives at least `'a`." The caller, when it calls this function, picks `'a` to be the shorter of the two input lifetimes. If the caller tries to use the returned reference past that, the borrow checker complains.

```rust
fn main() {
    let s1 = String::from("long string");
    let result;
    {
        let s2 = String::from("short");
        result = longest(s1.as_str(), s2.as_str());
        println!("{}", result);   // OK: still inside s2's scope.
    }
    // s2 dropped here.
    println!("{}", result);   // ERROR: result might point at s2's
                              // freed memory.
}
```

The compiler infers `'a` to be the inner block's scope (because that's the shorter of the two), and refuses to let you use `result` after the inner block ends.

### Lifetime elision

In practice, you rarely write lifetimes for simple functions. The compiler has rules for figuring them out:

* Each input reference parameter gets its own implicit lifetime.
* If there's exactly one input lifetime, it's also the output lifetime.
* If one of the inputs is `&self` or `&mut self`, that lifetime is the output lifetime.

So this works without explicit lifetimes:

```rust
fn first_word(s: &str) -> &str {
    // The compiler infers: for the lifetime of s, return a reference
    // that lives that long.
    let bytes = s.as_bytes();
    for (i, &b) in bytes.iter().enumerate() {
        if b == b' ' {
            return &s[..i];
        }
    }
    s
}
```

You only have to annotate when the elision rules can't decide on their own — usually multiple input references and an output reference, like `longest`.

### `'static`

There's one special lifetime called `'static`. It means "lives for the entire duration of the program." String literals have type `&'static str` because they're baked into the program's binary and never freed.

```rust
let s: &'static str = "hello, world";
// Lives forever.
```

People sometimes try to use `'static` as a hammer to fix lifetime errors. Resist. `'static` is a strong claim — "this reference will live forever." Usually you actually want a more specific lifetime; using `'static` masks bugs.

### Lifetimes are not generics-with-different-syntax

It's tempting to think `<'a>` and `<T>` are similar. They aren't. A type parameter `T` is filled in by the caller with a concrete type. A lifetime parameter `'a` is filled in by the compiler, by inference, from how the function is called. You don't write `longest::<'a, 'b>(...)` at the call site. The compiler picks `'a` for you.

## 1.8 Slices, `String` vs `&str`, `Vec` vs `&[T]`

Three terms come up constantly and confuse beginners:

* `String` — owned, growable, heap-allocated UTF-8 string. Like Java's `StringBuilder` if it also doubled as the regular String type.
* `&str` — borrowed string slice. A view into a string; doesn't own anything.
* `&'static str` — a string slice baked into the binary (string literals).

`String` is to `&str` as `Vec<T>` is to `&[T]`.

* `Vec<T>` — owned, growable, heap-allocated array.
* `&[T]` — borrowed slice. A view into an array; doesn't own anything.

Functions almost always want to take `&str` or `&[T]`, never `String` or `Vec<T>`. Why? Because `&str` is more general — it can refer to a `String`, a string literal, a piece of a `String`, etc. Taking a `String` would force the caller to either give up ownership or clone.

```rust
// GOOD: works with any string source.
fn count_chars(s: &str) -> usize {
    s.chars().count()
}

// BAD: forces caller to give up their String or clone.
fn count_chars_bad(s: String) -> usize {
    s.chars().count()
}

fn main() {
    let owned = String::from("hello");
    let literal = "world";

    count_chars(&owned);    // Works. &String coerces to &str.
    count_chars(literal);   // Works. Literal is already &str.
    count_chars(&owned[..2]);   // Works. Slice of owned.
}
```

This pattern — accept the borrowed slice form, return the owned form — is so ubiquitous it has a name: the **AsRef pattern**. Internalise it.

Mechanically, a `&str` is two things: a pointer to the start of some bytes, and a length. So 16 bytes total on a 64-bit machine. The bytes might be in a `String`'s heap buffer, or in the program's read-only data segment (string literals), or in the middle of another string. The slice doesn't care. It just knows where the bytes are and how many there are.

## 1.9 Common Compile Errors and How to Read Them

You will see these dozens of times in your first month. Learn to recognise them.

### "borrow of moved value"

```
error[E0382]: borrow of moved value: `s`
```

You used a value, then tried to use it again after a move. Either:

* Don't move it — pass `&s` instead of `s`.
* Clone it — `s.clone()`.
* Restructure the code to not need it after the move.

### "cannot borrow `x` as mutable, as it is not declared as mutable"

```
error[E0596]: cannot borrow `s` as mutable, as it is not declared as mutable
```

You wrote `let s = ...` and then tried to take `&mut s`. Variable bindings are immutable by default in Rust. Add `mut`: `let mut s = ...`.

### "cannot borrow `x` as mutable because it is also borrowed as immutable"

```
error[E0502]: cannot borrow `v` as mutable because it is also borrowed as immutable
```

The aliasing-XOR-mutability rule. There's a `&v` somewhere that's still alive when you try to do something that requires `&mut v`. Find the shared borrow and shorten its life (use it earlier, or factor it into a smaller scope).

### "cannot borrow `x` as mutable more than once at a time"

```
error[E0499]: cannot borrow `v` as mutable more than once at a time
```

You took `&mut v` twice. Only one allowed. Restructure.

### "lifetime may not live long enough"

```
error: lifetime may not live long enough
```

You returned or stored a reference, and the compiler can't prove it'll live long enough where the caller wants to use it. Either:

* Add explicit lifetime annotations to clarify your intent.
* Return an owned value (`String` instead of `&str`).
* Restructure so the reference doesn't need to escape.

### "use of moved value" inside a closure or loop

```rust
let s = String::from("hello");
for _ in 0..3 {
    println!("{}", s);   // OK: only borrows s.
}

let v = vec![1, 2, 3];
let f = move || println!("{:?}", v);   // moves v into the closure.
println!("{:?}", v);   // ERROR: v moved into f.
```

Closures capture variables either by reference (default) or by move (with `move ||`). If you `move`, you can't use the variable outside the closure anymore.

## 1.10 An Honest Word About the Learning Curve

Your first two weeks of writing Rust will involve a lot of confused staring at the compiler. You'll write something that looks obviously fine, and the compiler will reject it with a verbose, technical-sounding error. You'll google the error message, find a Stack Overflow answer that doesn't quite match your case, try things at random, and eventually stumble into something that compiles. You'll feel stupid.

Don't worry. Every Rust programmer went through this. The compiler errors are right — there is a real reason your code is unsafe — but the reason is often subtle and your mental model isn't equipped to see it yet.

A few tactical pieces of advice:

1. **Read the full error message.** The Rust compiler's errors are very good once you learn to read them. They include code snippets, arrows pointing at the problem, and often suggestions. People skim the error and miss half the information.
2. **Don't reach for `clone()` or `unsafe` to silence the borrow checker.** Cloning works but if you do it everywhere, you've defeated the point of Rust. `unsafe` is for very advanced cases. When the borrow checker rejects your code, the right response is "what is it telling me about my data flow that I should restructure?" Usually, restructuring teaches you something.
3. **Smaller functions help.** A function that does one thing has one set of borrows; the borrow checker has an easy time reasoning about it. A 200-line function with everything intertwined is a borrow-checker nightmare.
4. **Cargo Clippy.** Run `cargo clippy` regularly. It catches a huge range of stylistic and semantic issues that the compiler doesn't, and it teaches you idiomatic Rust as a side effect.

## 1.11 Summary: The Rules

1. **Memory is stack or heap.** Stack is fast and fixed-size. Heap is flexible but needs explicit management.
2. **Every value has one owner.** When the owner goes out of scope, the value is dropped.
3. **Assignment moves, by default.** After a move, the old binding is dead.
4. **`Copy` types are duplicated, not moved.** Integers, bools, etc. Custom structs are `Copy` only if you derive it.
5. **`.clone()` makes a deep copy explicitly.** It's visible in the source.
6. **`&T` is a shared (read-only) reference. `&mut T` is an exclusive (read-write) reference.**
7. **Aliasing XOR Mutability:** at any moment, you can have many `&T` *or* one `&mut T`, never both.
8. **References can't outlive the data they point to.** The borrow checker enforces this via lifetimes.
9. **Most of the time you don't write lifetime annotations.** When you do, it's because the elision rules are insufficient.
10. **Prefer `&str` and `&[T]` in function arguments.** Owned types should mostly be in return positions and at storage points.

## 1.12 Drill 1

Rules: show mechanism, not vibes. "I think it's a lifetime thing" gets zero credit. Reply with answers and I'll tear them apart.

**Q1. Mechanism.**

In your own words, explain why this code does not compile, and what's actually happening at the level of stack memory, heap memory, and pointers. Don't say "ownership." Say what bytes go where and what the compiler is preventing.

```rust
fn main() {
    let s1 = String::from("hello");
    let s2 = s1;
    println!("{}", s1);
}
```

You should be able to write at least 150 words and mention: the layout of `String` in memory, what `let s2 = s1` does at the byte level, what `drop` would do at the closing brace, and the specific bug that would result if the compiler permitted this code.

**Q2. Find every borrow error.**

Each of these snippets has at least one borrow checker error. For each, identify the error, name the rule it violates (move, aliasing-XOR-mutability, lifetime, dangling), and write a fix.

```rust
// (a)
fn main() {
    let s = String::from("hi");
    let t = s;
    println!("{} {}", s, t);
}

// (b)
fn main() {
    let mut v = vec![1, 2, 3];
    let r = &v[0];
    v.push(4);
    println!("{}", r);
}

// (c)
fn main() {
    let mut s = String::from("hi");
    let r1 = &mut s;
    let r2 = &mut s;
    r1.push_str("a");
    r2.push_str("b");
}

// (d)
fn first<'a>(v: &'a Vec<i32>) -> &'a i32 {
    &v[0]
}
fn main() {
    let r;
    {
        let v = vec![1, 2, 3];
        r = first(&v);
    }
    println!("{}", r);
}

// (e)
fn make_greeting() -> &str {
    let s = String::from("hello");
    &s
}
```

**Q3. The Copy question.**

Explain in your own words why `i32` is `Copy` but `String` isn't. What concrete bug would result if `String` were `Copy`? Use a stack-and-heap diagram in your explanation.

Then: define a struct `Point { x: i32, y: i32 }`. Is it `Copy` by default? If not, what one line do you add to make it `Copy`? What happens if you try to derive `Copy` on a struct that has a `String` field, and why?

**Q4. Implement and explain.**

Implement a function with this signature:

```rust
fn longest_line(text: &str) -> &str
```

It returns the longest line in the input (lines separated by `\n`). Tiebreaker: the first one. Empty input returns an empty slice.

Then explain, in writing:

* Why the return type `&str` is appropriate here, rather than `String`.
* What lifetime the compiler infers, and why it's the right one.
* How the function would change if you wanted to return an owned `String` instead, and what the cost difference is.

Test cases (must pass):

```
""                  -> ""
"hi"                -> "hi"
"a\nbb\nccc\nbb"    -> "ccc"
"foo\nbar\nbaz"     -> "foo"   (tiebreaker: first)
```

**Q5. Move-vs-borrow choices.**

For each of the following scenarios, decide whether the function should take `String`, `&String`, or `&str`, and explain why. The cost of getting this wrong in real code is forcing every caller to clone or restructure.

* (a) A function that prints the string to stdout.
* (b) A function that returns true if the string is a palindrome.
* (c) A function that stores the string in a struct field forever.
* (d) A function that returns a substring of the input.
* (e) A function that consumes the string, modifying it, and returns the modified version.

For each, say what you'd choose for the **return** type if the function returns a string-shaped thing.

**Q6. Reading.**

Read the chapter "What Is Ownership?" (chapter 4) of *The Rust Programming Language* book — it's free online at https://doc.rust-lang.org/book/ch04-00-understanding-ownership.html. Also read chapter 10.3 on lifetimes.

After reading, answer:

* What does the book's "two pointers to the same String" diagram illustrate, and how does Rust's move-on-assignment prevent the resulting bug?
* What is the difference between `String::from("hi")` and `"hi"` in terms of where the data lives?
* In what way are lifetimes "elided" rather than absent, in functions like `fn first_word(s: &str) -> &str`?

---

# Lesson 2: The Type System — Enums, Traits, and Error Handling

## 2.1 Why This Lesson Exists

In 1965, a British computer scientist named Tony Hoare introduced the **null reference** to the language ALGOL. Decades later, he called it "my billion-dollar mistake." The idea is simple: any pointer can either point at a thing, or be `null`. Reading through a null pointer crashes the program. In a few decades of computing, null pointer bugs have caused, by his estimate, on the order of a billion dollars in damages. They still crash production systems every day.

In 1989, the C++ standardisation committee added **exceptions** to the language. The idea: when something goes wrong, you `throw` an error, and it propagates up the call stack until something `catch`es it. Sounds elegant. The catch (no pun): every function call could potentially throw. The compiler doesn't tell you which ones. The function's signature doesn't say "this might throw." So you can't tell, by reading code, where the error paths are. This makes it nearly impossible to write robust C++ that handles every error correctly. Java and C# inherited the same problem. Go and Rust deliberately rejected it.

These two design mistakes — null and exceptions — are responsible for a huge chunk of bugs in modern software. Rust's type system is built around eliminating both. It does so through two features that, individually, exist in many languages, but combined create something distinctive:

* **Algebraic data types** (in Rust, `enum`) — types that represent a choice between alternatives. `Option<T>` and `Result<T, E>` are built from these.
* **Pattern matching with exhaustiveness** — when you handle an enum, the compiler insists you handle every case.

Together: nullable values are explicit (`Option<T>`), errors are explicit (`Result<T, E>`), and forgetting to handle either is a compile error.

This lesson covers Rust's type system in three parts: the data side (structs and enums), the behaviour side (traits and generics), and how they combine to produce idiomatic error handling. By the end you should be able to read and write the `Result`-and-`?` style of code that pervades real-world Rust.

## 2.2 Structs

Structs in Rust are like structs in C, classes in Java, or types in Go. They group named fields into a single type.

```rust
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
    is_buy: bool,
}

fn main() {
    let o = Order {
        id: 1,
        price: 10000,
        quantity: 5,
        is_buy: true,
    };
    println!("order {} is_buy={}", o.id, o.is_buy);
}
```

Notes:

* Field access is `.field`, like everywhere.
* Struct literals require all fields. There's no "zero value" like in Go.
* Fields are private to the module that defined them by default. To expose them, prefix with `pub`.

### Tuple structs and unit structs

```rust
// Tuple struct: like a struct, but fields have no names.
struct Point(f64, f64);

let p = Point(1.0, 2.0);
println!("{} {}", p.0, p.1);

// Unit struct: zero fields, used as a marker type.
struct AlwaysFresh;

let _x = AlwaysFresh;
```

Tuple structs are useful when the field meaning is obvious and naming would feel ceremonial — coordinates, wrappers, and so on. Unit structs are useful in advanced contexts (marker types, type-state programming).

### Methods via `impl`

Methods aren't part of the struct definition. They live in a separate `impl` block:

```rust
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
    is_buy: bool,
}

impl Order {
    // Associated function (no &self). Like a static method or constructor.
    fn new(id: u64, price: i64, quantity: i64, is_buy: bool) -> Self {
        Order { id, price, quantity, is_buy }
        //  ^^^^^ note: when field name and variable name match, you can
        //  use shorthand. This is `Order { id: id, price: price, ... }`.
    }

    // Method with shared self. Cannot mutate.
    fn notional(&self) -> i64 {
        self.price * self.quantity
    }

    // Method with exclusive self. Can mutate.
    fn reduce_quantity(&mut self, by: i64) {
        self.quantity -= by;
    }

    // Method that consumes self. Takes ownership; the caller can't
    // use the original after calling this.
    fn flip_side(self) -> Order {
        Order {
            is_buy: !self.is_buy,
            ..self   // ".." is "fill in the rest from this struct".
        }
    }
}

fn main() {
    let mut o = Order::new(1, 10000, 5, true);
    println!("{}", o.notional());      // method call
    o.reduce_quantity(2);
    let flipped = o.flip_side();       // moves o
    // o cannot be used here.
    println!("{}", flipped.is_buy);    // false
}
```

The three method receivers — `&self`, `&mut self`, `self` — correspond directly to the three borrow forms from Lesson 1. They tell the caller "I'll just look at it," "I need to modify it," or "give it to me, I'm taking ownership."

This is much more honest than other languages. In Java, you can't tell from the signature whether `list.add(x)` mutates the list (it does) or whether `list.size()` does (it doesn't). In Rust, you can:

```rust
list.add(x);     // method takes &mut self (mutates)
list.len();      // method takes &self (read-only)
```

The compiler enforces this. A method declared `&self` cannot mutate `self`.

## 2.3 Enums Are Sum Types

In C and Java, an `enum` is just a list of named integer constants:

```c
enum Color { RED = 0, GREEN = 1, BLUE = 2 };
```

Rust's `enum` is much more powerful. Each variant can carry its own data, of any type, in any shape.

```rust
enum Shape {
    Circle(f64),                    // variant carries a single f64 (radius)
    Rectangle { width: f64, height: f64 },   // named fields
    Triangle(f64, f64, f64),        // three sides
    Point,                          // no data
}
```

Mathematicians call this a **sum type** or **tagged union**. The value is *one of* the variants, and you know which one because the enum carries a hidden tag. Reading the data requires checking the tag — Rust's compiler enforces that you do this safely, via pattern matching.

```rust
fn area(s: Shape) -> f64 {
    match s {
        Shape::Circle(r) => 3.14159 * r * r,
        Shape::Rectangle { width, height } => width * height,
        Shape::Triangle(a, b, c) => {
            // Heron's formula
            let p = (a + b + c) / 2.0;
            (p * (p - a) * (p - b) * (p - c)).sqrt()
        },
        Shape::Point => 0.0,
    }
}
```

Two key properties:

**Exhaustiveness.** The `match` must handle every variant. If you forget `Shape::Point`, the compiler errors:

```
error[E0004]: non-exhaustive patterns: `Point` not covered
```

Add a new variant to the enum next year, and every `match` on it lights up with errors until you handle the new case. This is a *huge* deal for refactoring. Compare to Java's switch on an enum: forget a case, the program runs, the missing case silently falls through, and you find out in production.

**Per-variant data.** The `match` arm extracts the variant's data into named bindings. `Shape::Circle(r)` means "if it's a Circle, bind `r` to its inner f64." The compiler guarantees `r` is only accessible in that arm, where the variant tag is known to be Circle.

### Memory layout

Mechanically, a Rust enum is laid out as `(tag, data)`, where `data` is large enough to hold the biggest variant. The tag is usually 1 byte. The whole enum is the size of the largest variant plus the tag, rounded up for alignment. So `Shape` above is roughly 25 bytes (3×8 for `Triangle`, plus 1 for the tag, plus padding).

The compiler is also smart enough to avoid the tag in some cases. `Option<&T>` is the same size as `&T` (8 bytes) because `None` is represented by the all-zeros bit pattern, which a non-null reference can never have. This is called **niche optimisation**.

## 2.4 `Option<T>`: No Null

`Option<T>` is the most important enum in Rust. It's defined (roughly) as:

```rust
enum Option<T> {
    Some(T),
    None,
}
```

That's it. A value that's "either a `T` or nothing." The point is that **there is no null reference**. If a function might return nothing, its return type is `Option<T>`, and the caller cannot use the value without explicitly handling the None case.

```rust
fn find_user(id: u64) -> Option<String> {
    if id == 1 {
        Some(String::from("alice"))
    } else {
        None
    }
}

fn main() {
    let name = find_user(2);

    // We can't just print name. Its type is Option<String>, not String.
    // We have to handle both cases.

    match name {
        Some(n) => println!("found {}", n),
        None => println!("not found"),
    }
}
```

Forgetting `None` is a compile error. You cannot accidentally treat a missing value as if it were present.

### Common `Option` methods

In practice you don't always write `match`. The standard library has dozens of helper methods:

```rust
let x: Option<i32> = Some(5);

// Default if None.
let y = x.unwrap_or(0);              // 5

// Compute default if None.
let y = x.unwrap_or_else(|| 0);

// Crash if None. Use sparingly. Useful in tests and prototypes.
let y = x.unwrap();                  // 5
let y: Option<i32> = None;
let y = y.unwrap();                  // PANIC: called `unwrap()` on a `None` value

// Crash with a custom message.
let y = x.expect("x must be Some by this point");

// Map over the inner value if Some.
let y = x.map(|v| v * 2);            // Some(10)

// Chain another Option-returning operation.
let y = x.and_then(|v| if v > 0 { Some(v) } else { None });

// Boolean tests.
let b = x.is_some();                 // true
let b = x.is_none();                 // false
```

`unwrap` and `expect` panic on `None`. Panicking is Rust's "the program is broken, abort" mechanism. It's appropriate when a `None` here means a bug in your code, not a possible runtime condition you should handle. In production code you should rarely use `unwrap`; in tests and prototypes it's fine.

### `if let`

For the common case of "do something if Some, ignore if None," there's a shorthand:

```rust
if let Some(n) = name {
    println!("found {}", n);
}
// Equivalent to:
//   match name {
//       Some(n) => println!("found {}", n),
//       None => {},
//   }
```

`if let` is also `else`-able:

```rust
if let Some(n) = name {
    println!("found {}", n);
} else {
    println!("not found");
}
```

## 2.5 `Result<T, E>`: No Exceptions

`Result<T, E>` is `Option`'s big sibling, used for fallible operations:

```rust
enum Result<T, E> {
    Ok(T),
    Err(E),
}
```

A function that might fail returns `Result<T, E>`. The caller has to handle both arms. Errors are part of the type signature, visible at every call site.

```rust
use std::num::ParseIntError;

fn parse_age(s: &str) -> Result<u32, ParseIntError> {
    s.parse::<u32>()
}

fn main() {
    match parse_age("25") {
        Ok(n) => println!("age is {}", n),
        Err(e) => println!("invalid: {}", e),
    }

    match parse_age("not a number") {
        Ok(n) => println!("age is {}", n),
        Err(e) => println!("invalid: {}", e),
    }
}
```

Compare to Java:

```java
try {
    int age = Integer.parseInt("not a number");
    System.out.println("age is " + age);
} catch (NumberFormatException e) {
    System.out.println("invalid");
}
```

The Java version's signature `int parseInt(String)` says nothing about the exception. You only learn it can fail by reading documentation, or by your tests crashing in production when someone hands it bad input. The Rust signature `fn parse(s: &str) -> Result<u32, ParseIntError>` makes the failure part of the type. The compiler will not let you treat the result as a u32 without first dealing with the error.

### The `?` operator

In real code, you often have a chain of fallible operations and you want to bail out of the whole function if any step fails. Writing `match` for each is tedious. The `?` operator does exactly this in one character:

```rust
use std::fs;
use std::io;

fn read_and_double_first_number(path: &str) -> Result<i64, io::Error> {
    let contents = fs::read_to_string(path)?;
    //                                     ^
    //  if Err, return that Err immediately from this function.
    //  if Ok, unwrap and continue.

    let first_line = contents.lines().next().unwrap_or("");
    let n: i64 = first_line.parse().unwrap_or(0);
    Ok(n * 2)
}
```

Read the `?` as: "if this is `Err`, return that `Err` from the enclosing function; otherwise unwrap to the `Ok` value and keep going." It's a short-circuit, like exceptions but visible. You can see exactly where the function might exit early.

Three things to note:

* `?` only works in functions that themselves return `Result` (or `Option`).
* `?` does an automatic conversion via the `From` trait — if the function returns `Result<_, MyError>` and the `?` is on a `Result<_, OtherError>`, the compiler will try `MyError::from(OtherError)`. We'll come back to this.
* You'll see `?` everywhere in real Rust. Get comfortable.

### Result methods

Like `Option`, `Result` has many helper methods:

```rust
let r: Result<i32, &str> = Ok(5);

let v = r.unwrap();              // 5; panics on Err
let v = r.unwrap_or(0);          // 5
let v = r.unwrap_or_else(|e| 0); // 5

// Map over the Ok value.
let r2 = r.map(|v| v * 2);       // Ok(10)

// Map over the Err value.
let r3: Result<i32, String> = r.map_err(|e| String::from(e));

// Chain another Result-returning operation.
let r4 = r.and_then(|v| if v > 0 { Ok(v) } else { Err("neg") });

let b = r.is_ok();
let b = r.is_err();
```

## 2.6 Traits: Behaviour over Types

A **trait** is Rust's mechanism for abstracting over behaviour. It says "any type that implements this trait must provide these methods." It's roughly like:

* A Java `interface`.
* A Go `interface`.
* A C++ pure virtual class.
* A Haskell type class.

But more powerful than the first three, and more explicit than the second.

### Defining and implementing

```rust
trait Greeting {
    fn hello(&self) -> String;
}

struct English;
struct German;

impl Greeting for English {
    fn hello(&self) -> String {
        String::from("hello")
    }
}

impl Greeting for German {
    fn hello(&self) -> String {
        String::from("hallo")
    }
}

fn main() {
    let e = English;
    let g = German;
    println!("{}", e.hello());
    println!("{}", g.hello());
}
```

Difference from Go: in Go, types implement interfaces implicitly — if a type happens to have the required methods, it satisfies the interface, no declaration needed. In Rust, you write `impl Greeting for English`, an explicit declaration. The Rust style is preferred (in my opinion) because adding a method to a type can't accidentally make it satisfy a new interface and change behaviour somewhere unrelated.

### Default methods

A trait can provide default method bodies. Implementors can override or take the default.

```rust
trait Greeting {
    fn hello(&self) -> String;

    fn greet_loudly(&self) -> String {
        // Default implementation. Implementors can override or just use this.
        self.hello().to_uppercase() + "!"
    }
}
```

### Common standard-library traits

A handful of traits show up everywhere. Memorise these:

* **`Debug`** — `println!("{:?}", x)`. Programmer-readable representation. Almost everything should derive this.
* **`Display`** — `println!("{}", x)`. User-readable representation. Implement when it makes sense.
* **`Clone`** — `.clone()` makes a deep copy. Most types should derive this.
* **`Copy`** — implicit duplication on assignment, for cheap types.
* **`PartialEq` / `Eq`** — `==` and `!=`. The "Partial" handles types like `f64` where `NaN != NaN` (so equality isn't reflexive).
* **`PartialOrd` / `Ord`** — `<`, `<=`, `>`, `>=`, sortable.
* **`Hash`** — usable as a `HashMap` key.
* **`Default`** — provides a `default()` constructor that returns a "zero" value.
* **`Iterator`** — produces a sequence of values. The whole iteration story is built on this.
* **`From` / `Into`** — type conversions. `String::from("hi")` and `"hi".into()`.

### Deriving

For the most common traits, you don't write the impl by hand; you ask the compiler to generate it via the `derive` attribute:

```rust
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
}

fn main() {
    let o = Order { id: 1, price: 100, quantity: 5 };
    let o2 = o.clone();
    println!("{:?}", o);     // works because of Debug derive
    assert_eq!(o, o2);       // works because of PartialEq derive
}
```

The derive expands at compile time into the obvious implementation. For `Debug`, it prints the struct name and each field. For `Clone`, it clones each field. For `PartialEq`, it compares each field. You almost always derive all of `Debug`, `Clone`, `PartialEq` on data structs, and `Eq`, `Hash` if the type has no floats.

## 2.7 Generics

Generics let you write a function or type that works for many types.

```rust
fn largest<T: PartialOrd>(list: &[T]) -> &T {
    let mut largest = &list[0];
    for item in list {
        if item > largest {
            largest = item;
        }
    }
    largest
}

fn main() {
    let ints = vec![34, 50, 25, 100, 65];
    println!("{}", largest(&ints));

    let chars = vec!['y', 'm', 'a', 'q'];
    println!("{}", largest(&chars));
}
```

Notes:

* `<T: PartialOrd>` declares a type parameter `T` with the **bound** that `T` must implement `PartialOrd`. Without the bound, you couldn't use `>` on a `T`.
* The function takes `&[T]` (a slice of T) and returns `&T` (a reference into the slice). Lifetime is elided; the compiler infers it.

### Trait bounds, multiple bounds, where clauses

```rust
// Single bound.
fn print_summary<T: Display>(item: T) {
    println!("{}", item);
}

// Multiple bounds.
fn print_and_clone<T: Display + Clone>(item: T) {
    let dup = item.clone();
    println!("{} {}", item, dup);
}

// Many bounds get unwieldy. Use a `where` clause.
fn complicated<T, U>(t: T, u: U) -> i32
where
    T: Display + Clone,
    U: Clone + Debug,
{
    // ...
    0
}
```

### Generic structs

```rust
struct Pair<T> {
    first: T,
    second: T,
}

impl<T: PartialOrd> Pair<T> {
    fn larger(&self) -> &T {
        if self.first > self.second { &self.first } else { &self.second }
    }
}

fn main() {
    let p = Pair { first: 1, second: 2 };
    println!("{}", p.larger());
}
```

### Monomorphisation

Rust's generics don't have runtime cost. The compiler generates a separate copy of the function for each concrete type used. `largest::<i32>` and `largest::<char>` are two distinct functions in the final binary, each specialised to its type. This is called **monomorphisation**.

The benefit: zero runtime overhead. A generic function in Rust is exactly as fast as the hand-written specialised version.

The cost: code size. Each instantiation adds machine code. If you instantiate a big generic function with 50 different types, you get 50 copies.

C++ templates work the same way, with the same tradeoff. Java generics work differently — they erase to `Object` at compile time and use casts at runtime. Rust's approach is faster but produces bigger binaries.

## 2.8 Trait Objects: Dynamic Dispatch

Sometimes you don't want monomorphisation. You want a heterogeneous collection — a `Vec<Animal>` containing dogs, cats, and rabbits. With generics, you can't, because each `Animal` would be a distinct type. You need **dynamic dispatch**.

```rust
trait Animal {
    fn speak(&self) -> String;
}

struct Dog;
struct Cat;

impl Animal for Dog {
    fn speak(&self) -> String { String::from("woof") }
}
impl Animal for Cat {
    fn speak(&self) -> String { String::from("meow") }
}

fn main() {
    // Vec<Box<dyn Animal>>: a vector of pointers to things that implement Animal.
    let animals: Vec<Box<dyn Animal>> = vec![
        Box::new(Dog),
        Box::new(Cat),
    ];

    for a in &animals {
        println!("{}", a.speak());
    }
}
```

`dyn Animal` is a **trait object**. At runtime, each `Box<dyn Animal>` is two pointers: one to the data, one to a vtable (a small table of function pointers, one per trait method). `a.speak()` looks up `speak` in the vtable and calls it.

This is the same machinery as Java's interface dispatch, or C++'s virtual functions. It costs an indirect call (5–10 ns), and it prevents inlining.

### When to use generics vs trait objects

Use generics (`<T: Animal>`) when:

* You know the concrete type at compile time.
* You want maximum performance.
* Code size is acceptable.
* Each call site uses one concrete type.

Use trait objects (`dyn Animal`) when:

* You want a heterogeneous collection.
* You want dynamic plugin-like behaviour at runtime.
* You're willing to pay 5–10 ns per call for the indirection.

A useful instinct: write generics by default. Reach for `dyn` when you genuinely need polymorphism at runtime.

## 2.9 Pattern Matching, in Depth

We've used `match` informally. Here are the patterns that matter.

### Matching enums (covered above)

```rust
match shape {
    Shape::Circle(r) => 3.14 * r * r,
    Shape::Rectangle { width, height } => width * height,
    _ => 0.0,   // catch-all wildcard
}
```

`_` matches anything and binds nothing.

### Matching literals and ranges

```rust
let n = 5;
match n {
    0 => println!("zero"),
    1 | 2 | 3 => println!("small"),     // alternation
    4..=10 => println!("medium"),       // inclusive range
    _ => println!("big"),
}
```

### Destructuring

```rust
struct Point { x: i32, y: i32 }
let p = Point { x: 3, y: 4 };

match p {
    Point { x: 0, y: 0 } => println!("origin"),
    Point { x, y: 0 } => println!("on x-axis at {}", x),
    Point { x: 0, y } => println!("on y-axis at {}", y),
    Point { x, y } => println!("at ({}, {})", x, y),
}
```

Tuples work the same way:

```rust
let pair = (3, 4);
match pair {
    (0, _) => println!("x is zero"),
    (_, 0) => println!("y is zero"),
    (x, y) => println!("({}, {})", x, y),
}
```

### Guards

A `match` arm can have an extra `if` condition:

```rust
match age {
    n if n < 0 => println!("invalid"),
    n if n < 18 => println!("minor"),
    n if n < 65 => println!("adult"),
    _ => println!("senior"),
}
```

### `if let` and `while let`

```rust
// Print only the Some values.
if let Some(n) = some_option {
    println!("{}", n);
}

// Pop from a stack until empty.
let mut stack = vec![1, 2, 3];
while let Some(top) = stack.pop() {
    println!("{}", top);
}
```

`while let` is great for state machines, parsers, and queue-draining loops.

## 2.10 Idiomatic Error Handling

Putting all of the above together, here's how you handle errors in real Rust.

### Step 1: define your error type as an enum

For a small program, you can use `Box<dyn std::error::Error>` and not bother defining your own. For anything serious, you define a custom enum.

```rust
use std::io;
use std::num::ParseIntError;

#[derive(Debug)]
enum AppError {
    Io(io::Error),
    Parse(ParseIntError),
    InvalidInput(String),
}

impl std::fmt::Display for AppError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AppError::Io(e) => write!(f, "io error: {}", e),
            AppError::Parse(e) => write!(f, "parse error: {}", e),
            AppError::InvalidInput(s) => write!(f, "invalid input: {}", s),
        }
    }
}

impl std::error::Error for AppError {}
```

### Step 2: implement `From` for each underlying error type

This is what makes `?` work. When `?` sees a `Result<_, io::Error>` in a function that returns `Result<_, AppError>`, it calls `AppError::from(io_error)`.

```rust
impl From<io::Error> for AppError {
    fn from(e: io::Error) -> Self { AppError::Io(e) }
}

impl From<ParseIntError> for AppError {
    fn from(e: ParseIntError) -> Self { AppError::Parse(e) }
}
```

### Step 3: write fallible code naturally with `?`

```rust
use std::fs;

fn parse_first_number(path: &str) -> Result<i64, AppError> {
    let contents = fs::read_to_string(path)?;     // io::Error -> AppError
    let first = contents.lines().next()
        .ok_or_else(|| AppError::InvalidInput("empty file".to_string()))?;
    let n = first.parse::<i64>()?;                // ParseIntError -> AppError
    Ok(n)
}

fn main() {
    match parse_first_number("data.txt") {
        Ok(n) => println!("first number: {}", n),
        Err(e) => eprintln!("error: {}", e),
    }
}
```

Read this carefully. `?` short-circuits on error. Each `?` automatically converts to `AppError` via `From`. The body reads almost like exception-based code, but every error path is visible (the `?` markers) and every error type is in the function's signature.

### `thiserror` and `anyhow`

Writing the boilerplate above by hand gets tedious. Two community crates ease the pain:

* **`thiserror`** — a derive macro for defining error enums. Saves dozens of lines of `Display` and `From` impls.
* **`anyhow`** — a generic error type for application code where you just want "something went wrong" and a backtrace, without enumerating cases.

A typical `thiserror`-based error type:

```rust
use thiserror::Error;

#[derive(Error, Debug)]
enum AppError {
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),

    #[error("parse error: {0}")]
    Parse(#[from] std::num::ParseIntError),

    #[error("invalid input: {0}")]
    InvalidInput(String),
}
```

That's it. `#[from]` gives you the `From` impl. `#[error("...")]` gives you the `Display`. `Error` and `Debug` are derived.

The community convention: use `thiserror` for libraries where callers need to inspect errors; use `anyhow` for binaries where you just want to report.

### Panic vs Result

Rust has two error mechanisms, not one. `Result` is for *expected* failures — file not found, malformed input, network down. `panic!` is for *unrecoverable bugs* — index out of bounds, unwrap on None, an invariant violation that means your program is already broken.

Panic doesn't propagate via `Result`. It unwinds the stack (calling drops along the way) and either aborts the process or, if you catch it with `std::panic::catch_unwind`, gives you back the chance to recover. Most code shouldn't catch panics; it should let the process die and let the supervisor restart it.

A useful rule: **if a sane caller might want to handle this case, return `Result`. If the only sensible response is "something is broken, stop," panic.**

## 2.11 Summary: The Rules

1. **Structs group named fields. Methods live in `impl` blocks, with `&self`, `&mut self`, or `self`.**
2. **Enums are sum types.** Variants can carry their own data. Pattern matching is exhaustive.
3. **No null. Use `Option<T>`.**
4. **No exceptions. Use `Result<T, E>` and `?`.**
5. **Traits define behaviour. Implementing a trait is explicit (`impl Trait for Type`).**
6. **Derive the common traits** (`Debug`, `Clone`, `PartialEq`, `Eq`, `Hash`) on data types where they apply.
7. **Generics are zero-cost.** Each instantiation produces a specialised copy at compile time.
8. **Trait objects (`dyn Trait`) give runtime polymorphism at the cost of an indirect call.**
9. **For real programs, define a custom error enum.** Use `thiserror` to cut boilerplate.
10. **`?` propagates errors with automatic conversion via `From`.**
11. **Panic for bugs. `Result` for expected failures.**

## 2.12 Drill 2

**Q1. Translate from Java/Go.**

Translate the following pseudo-Java into idiomatic Rust. You must use `Result`, `?`, and a custom error enum. No `unwrap` outside of tests.

```java
public int processFile(String path) throws IOException {
    String contents = Files.readString(Paths.get(path));
    String[] lines = contents.split("\n");
    if (lines.length == 0) {
        throw new IllegalArgumentException("empty file");
    }
    int total = 0;
    for (String line : lines) {
        if (line.isEmpty()) continue;
        total += Integer.parseInt(line);
    }
    return total;
}
```

After writing the Rust version, answer:

* Where in your code can the function return early due to an error? List every site.
* What is the equivalent of `IllegalArgumentException` in your design?
* If a single line is malformed, what happens, and is that the same as the Java behaviour?

**Q2. Build an enum that models real choice.**

Define an enum `Event` representing events on an exchange:

* `OrderPlaced { id: u64, price: i64, qty: i64, side: Side }`
* `OrderCancelled { id: u64 }`
* `Trade { taker: u64, maker: u64, price: i64, qty: i64 }`
* `MarketClosed`

Where `Side` is itself an enum: `Buy` or `Sell`.

Then write a function `summarise(events: &[Event]) -> String` that produces a one-line summary like:

```
"3 placed, 1 cancelled, 2 trades, market open"
```

Use a single `match` per event in the loop body. Then add a new variant `Event::OrderModified { id: u64, new_price: i64 }` to the enum and observe what the compiler tells you. Fix it. Note in your answer what would have happened in a language without exhaustiveness checking.

**Q3. Generics vs trait objects.**

Implement two versions of a sorting function. Both should sort a slice of any type that implements `Ord`.

```rust
// Version 1: generic.
fn sort_generic<T: Ord>(items: &mut [T]) { ... }

// Version 2: trait object.
fn sort_dyn(items: &mut [Box<dyn Ord>]) { ... }
```

The second one will not compile as-is. Why? (Hint: there's a concept called "object safety." Look it up.) Without solving it via `dyn`, what's a cleaner way to support heterogeneous comparison if you really needed it?

Then: write a benchmark using the `criterion` crate (or a manual `Instant::now()` timing loop) sorting 100,000 i32s. Compare the generic version to:

* `sort_dyn` after you make it work somehow.
* `Vec::sort` from the standard library.

Report ns per element. Explain the differences in terms of dynamic dispatch and inlining.

**Q4. The lifetime + trait combo.**

Implement this trait and structure:

```rust
trait Tokeniser {
    fn next_token<'a>(&mut self, input: &'a str) -> Option<&'a str>;
}

struct WordTokeniser {
    pos: usize,
}
```

`WordTokeniser` should yield successive whitespace-separated words from the input string, each as a `&str` slice that borrows from the input. The position state is per-tokeniser, so calling `next_token` repeatedly with the same input advances through it.

Test:

```rust
let mut t = WordTokeniser { pos: 0 };
let s = "alpha beta gamma";
assert_eq!(t.next_token(s), Some("alpha"));
assert_eq!(t.next_token(s), Some("beta"));
assert_eq!(t.next_token(s), Some("gamma"));
assert_eq!(t.next_token(s), None);
```

Then explain in your own words: why does `next_token`'s output lifetime have to come from the input parameter, not from `&mut self`? What would go wrong if you tried `fn next_token<'a>(&'a mut self, input: &str) -> Option<&'a str>`?

**Q5. Find every mistake.**

The following code compiles but has at least four issues with idiomatic style or correctness. Find and explain each.

```rust
fn parse_config(path: String) -> i32 {
    let s = std::fs::read_to_string(&path).unwrap();
    let parts: Vec<&str> = s.split(',').collect();
    let n: i32 = parts[0].parse().unwrap();
    return n;
}
```

For each issue, write the corrected version and explain why it's an improvement.

**Q6. Reading.**

Read these chapters of *The Rust Programming Language*:

* Chapter 6: Enums and Pattern Matching.
* Chapter 9: Error Handling.
* Chapter 10: Generic Types, Traits, and Lifetimes.

After reading, answer:

* What does the book mean when it says `Option<T>` and `Result<T, E>` are "encoded into the type system"? Compare to how nullability and exceptions are encoded in Java.
* The book describes the difference between recoverable and unrecoverable errors. Give one example of each from a domain you're familiar with (web, games, embedded, whatever).
* What is the orphan rule, and why does it exist? (Searching "Rust orphan rule" is fair.)

---

# Lesson 3: Concurrency and Shared State

## 3.1 Why This Lesson Exists

In 1995, Java shipped with built-in support for threads, mutexes, and shared memory. It was a tour de force — multi-threaded programming became accessible to mainstream developers. Then everyone discovered, painfully and over many years, that multi-threaded shared-memory programming is *extremely hard to do correctly*. Threads need synchronisation; getting synchronisation wrong gives you data races; data races cause bugs that appear once a month in production and never on your laptop. The Java Memory Model paper that tried to define what was even *legal* in concurrent Java is dozens of pages of subtle rules that almost no working programmer fully understands.

Go shipped goroutines and channels in 2009 with a different bet: "share memory by communicating, don't communicate by sharing memory." Channels are great. But Go didn't, and still doesn't, prevent you from sharing memory directly between goroutines. Slap a `sync.Mutex` on a struct and have at it. Forget the mutex on one access path and you have a race condition. Go ships with a race detector, which is excellent, but it only finds races your tests actually trigger.

Rust takes a stronger position: **the type system prevents data races at compile time**. The same ownership rules from Lesson 1 — aliasing-XOR-mutability — turn out to be exactly the rules you need for safe concurrency. Rust extends them with two marker traits, `Send` and `Sync`, and a small kit of synchronisation primitives. The result: you can write multi-threaded code with the confidence that if it compiles, it has no data races. You can still have logical bugs (deadlocks, lost updates) but the entire category of "two threads tore my data structure in half" is structurally impossible.

This lesson covers: why simple sharing fails, the smart pointers Rust provides for shared ownership (`Box`, `Rc`, `Arc`, `RefCell`, `Mutex`), how `Send` and `Sync` work, threads and channels, and a quick orientation to async/await. By the end, you should be able to read concurrent Rust code and know which sharing primitive each line is using and why.

## 3.2 Why You Can't Just Share

Let's see what happens if we naively try to share data between threads:

```rust
use std::thread;

fn main() {
    let v = vec![1, 2, 3];
    let handle = thread::spawn(|| {
        println!("{:?}", v);     // borrow of v from outside the thread
    });
    handle.join().unwrap();
}
```

This fails to compile:

```
error[E0373]: closure may outlive the current function, but it borrows `v`,
              which is owned by the current function
```

The issue: the spawned thread might keep running after `main` returns. If `main` returns and `v` is dropped, the thread is left holding a reference to freed memory. Use-after-free.

The fix is to **move** `v` into the closure:

```rust
let v = vec![1, 2, 3];
let handle = thread::spawn(move || {
    println!("{:?}", v);
});
handle.join().unwrap();
```

`move ||` says "take ownership of every captured variable." `v` is moved into the thread; the original `main` no longer has access. The thread can hold onto `v` for as long as it wants.

But what if we want **two** threads to access `v`? Now we have a problem. Ownership says each value has one owner. We can't move `v` into both closures.

We need a way to express **shared ownership**. Enter smart pointers.

## 3.3 Smart Pointers: A Tour

A **smart pointer** in Rust is a struct that holds a pointer to data plus some additional bookkeeping, and implements traits (`Deref`, `Drop`) that let it behave like a pointer with extra rules. There are several, each with a precise role.

### `Box<T>`: heap allocation

The simplest. `Box<T>` puts a `T` on the heap and gives you a pointer to it.

```rust
let b: Box<i32> = Box::new(42);
println!("{}", *b);   // dereference to get the i32
```

When `b` goes out of scope, the heap memory is freed. Same single-owner rules as everything else.

You use `Box`:

* When a value is too big for the stack (recursive data structures, large structs).
* When you want to store a trait object: `Box<dyn Trait>`.
* When you want to transfer ownership of a heap allocation cheaply (moving a `Box` is just moving 8 bytes).

```rust
// Recursive type — can't be on stack because the size would be infinite.
enum List {
    Cons(i32, Box<List>),
    Nil,
}

let list = List::Cons(1, Box::new(List::Cons(2, Box::new(List::Nil))));
```

### `Rc<T>`: reference counting (single-threaded)

`Rc` stands for "reference counted." It allows multiple owners of the same data on the same thread.

```rust
use std::rc::Rc;

let a = Rc::new(String::from("hello"));
let b = Rc::clone(&a);    // increments the reference count to 2
let c = Rc::clone(&a);    // count is 3

// All three (a, b, c) point at the same String. None of them owns it
// outright; they share it. When the last one is dropped, the String
// is finally freed.
```

Mechanically: `Rc<T>` allocates a small heap block containing a counter and the `T`. `Rc::clone` increments the counter and returns another `Rc`. When an `Rc` is dropped, it decrements the counter; when the counter hits zero, the `T` is dropped.

Two crucial properties:

* `Rc<T>` only allows shared (immutable) access. You can read the inner `T` through any `Rc`, but you can't mutate it.
* `Rc<T>` is **not thread-safe**. The reference count is updated non-atomically. If two threads both clone or drop an `Rc` at the same time, the count goes wrong, leading to use-after-free or double-free.

`Rc` is for "I have a complex graph of objects sharing references on a single thread." A typical example is a tree where children might be referenced by multiple parents.

### `Arc<T>`: atomic reference counting (multi-threaded)

`Arc` is `Rc`'s thread-safe sibling. Same idea, but the reference count is atomic, so it's safe to clone and drop across threads.

```rust
use std::sync::Arc;
use std::thread;

let v = Arc::new(vec![1, 2, 3, 4, 5]);

let handles: Vec<_> = (0..3).map(|i| {
    let v = Arc::clone(&v);   // each thread gets its own Arc
    thread::spawn(move || {
        println!("thread {}: {:?}", i, v);
    })
}).collect();

for h in handles {
    h.join().unwrap();
}
```

`Arc<T>` costs slightly more than `Rc<T>` (atomic increments and decrements are 5–10 ns more expensive than non-atomic ones). Use `Rc` when you know the data is single-threaded. Use `Arc` when it crosses threads. The compiler will tell you if you got it wrong.

Same property as `Rc`: `Arc<T>` only gives you shared access. To mutate, you need to combine `Arc` with one of the next pieces.

### `RefCell<T>`: interior mutability (single-threaded)

Sometimes you need to mutate data through a shared reference. The borrow checker normally forbids this, but there's a controlled escape hatch: `RefCell<T>`.

```rust
use std::cell::RefCell;

let c = RefCell::new(5);

let r = c.borrow();          // gives back a shared (Ref) reference; like &i32
println!("{}", *r);
drop(r);                     // explicit drop to end the borrow

*c.borrow_mut() += 10;        // exclusive (RefMut) reference; like &mut i32
println!("{}", c.borrow());   // 15
```

`RefCell` enforces the borrow rules **at runtime** instead of compile time. You can have multiple `borrow()`s, or one `borrow_mut()`, but never both at once. If you violate this — call `borrow_mut()` while a `borrow()` is outstanding — `RefCell` panics:

```rust
let c = RefCell::new(5);
let _r1 = c.borrow();
let _r2 = c.borrow_mut();    // PANIC: already borrowed: BorrowMutError
```

`RefCell` is for when you really do have correct logic but the borrow checker can't see it. It moves the check from compile time to runtime, which is slower (a couple of branches per borrow) and less safe (panics instead of compile errors). Use sparingly.

`RefCell` is single-threaded. Its multi-threaded equivalent is `Mutex`, which you'll meet in a moment.

### Combining: `Rc<RefCell<T>>` and `Arc<Mutex<T>>`

These two combinations are *the* idiomatic shared-mutable-state patterns.

* **`Rc<RefCell<T>>`** — multiple owners on a single thread, with mutation. Common in graph-like data structures.
* **`Arc<Mutex<T>>`** — multiple owners across threads, with mutation. The default for shared mutable state in concurrent Rust.

```rust
use std::sync::{Arc, Mutex};
use std::thread;

let counter = Arc::new(Mutex::new(0));

let handles: Vec<_> = (0..10).map(|_| {
    let counter = Arc::clone(&counter);
    thread::spawn(move || {
        let mut num = counter.lock().unwrap();
        *num += 1;
        // num is dropped at the end of this block, releasing the lock.
    })
}).collect();

for h in handles {
    h.join().unwrap();
}

println!("counter: {}", *counter.lock().unwrap());   // 10, deterministically
```

Read this carefully:

* `Arc<Mutex<i32>>` is a *shared, lockable integer*.
* `counter.lock().unwrap()` acquires the lock. Returns a `MutexGuard<i32>`, which acts like `&mut i32` and releases the lock when dropped.
* `unwrap()` is here because `lock()` returns `Result<_, PoisonError>` — if some thread panicked while holding the lock, the lock is "poisoned." The `unwrap` propagates the panic.

The whole thing is impossible to misuse. You can't access the inner value without locking. The lock is released automatically when the guard goes out of scope. You can't double-lock from the same thread (you'd deadlock; `Mutex` is not reentrant). You cannot forget to lock, the way you can in Java/Go.

## 3.4 `Send` and `Sync`: How the Compiler Knows

This is the deep magic. Rust has two marker traits — traits with no methods, just used to label types:

* **`Send`** — "values of this type are safe to transfer ownership of to another thread."
* **`Sync`** — "values of this type are safe to share by reference across threads. Equivalently: `T: Sync` iff `&T: Send`."

Almost every type is `Send` and `Sync`. Specifically:

* Primitives: `i32`, `bool`, etc., are both.
* `String`, `Vec<T>` (for `T: Send`), `Box<T>` — all `Send + Sync`.
* `Arc<T>` is `Send + Sync` if `T: Send + Sync`.
* `Mutex<T>` is `Send + Sync` if `T: Send`. (Note: only `Send`, not `Sync` — the Mutex provides the synchronisation.)
* **`Rc<T>` is neither `Send` nor `Sync`** — its reference count is not atomic.
* **`RefCell<T>` is `Send` (if T is) but not `Sync`** — its runtime borrow check is not atomic.
* **`*const T` and `*mut T` (raw pointers)** are neither, by default.

These traits are auto-implemented by the compiler based on a type's fields. A struct is `Send` if all its fields are `Send`. So if you put an `Rc` inside your struct, your struct is also not `Send`, and the compiler will refuse to send it across a thread boundary.

This is how `thread::spawn` is defined:

```rust
pub fn spawn<F, T>(f: F) -> JoinHandle<T>
where
    F: FnOnce() -> T + Send + 'static,
    T: Send + 'static,
```

The closure must be `Send` (transferable to another thread) and `'static` (no references with shorter lifetimes). The return type must also be `Send`. If you try to capture an `Rc` in a thread closure, the compiler refuses:

```
error[E0277]: `Rc<i32>` cannot be sent between threads safely
```

Read it as a positive: the compiler caught your bug at compile time. In Java or Go, you'd have shipped this and discovered the data race in production a month later.

This is the central insight of Rust concurrency. The same ownership rules that prevent use-after-free in single-threaded code, extended with `Send` and `Sync`, prevent data races in multi-threaded code.

## 3.5 Threads, in Practice

```rust
use std::thread;
use std::time::Duration;

fn main() {
    let handle = thread::spawn(|| {
        for i in 1..5 {
            println!("from spawned thread: {}", i);
            thread::sleep(Duration::from_millis(50));
        }
    });

    for i in 1..3 {
        println!("from main thread: {}", i);
        thread::sleep(Duration::from_millis(50));
    }

    handle.join().unwrap();
}
```

`thread::spawn` creates an OS thread. Unlike Go's goroutines, these are *real* OS threads — relatively heavy (megabytes of stack each), and you don't typically have millions of them. For lots-of-small-tasks workloads (web servers handling thousands of concurrent connections), the idiomatic Rust answer is `async`/`await` with a runtime like Tokio, not OS threads.

`handle.join()` waits for the thread to finish and returns its result. It returns `Result<T, Box<dyn Any>>` because the thread might have panicked; the `Err` arm carries the panic value. Most code just calls `.unwrap()`.

### Scoped threads

In modern Rust (since 1.63), you can spawn threads that *borrow* from the surrounding stack frame, as long as they finish before the borrow ends. This avoids needing `Arc` for shared read-only data.

```rust
use std::thread;

let v = vec![1, 2, 3];

thread::scope(|s| {
    s.spawn(|| {
        println!("{:?}", v);   // borrows v
    });
    s.spawn(|| {
        println!("{}", v.len());   // also borrows v
    });
    // scope blocks here until all spawned threads finish.
});

// We can use v normally afterwards. It was just borrowed.
println!("{:?}", v);
```

`thread::scope` is much nicer than `Arc` for cases where threads have a bounded lifetime tied to a parent.

## 3.6 Channels: Communicating Between Threads

Rust's standard library provides channels in `std::sync::mpsc` (multi-producer, single-consumer):

```rust
use std::sync::mpsc;
use std::thread;

fn main() {
    let (tx, rx) = mpsc::channel();

    for i in 0..5 {
        let tx = tx.clone();   // each producer needs its own sender
        thread::spawn(move || {
            tx.send(i).unwrap();
        });
    }
    drop(tx);   // drop the original sender; the channel closes when all senders are dropped.

    for received in rx {
        println!("got: {}", received);
    }
}
```

Notes:

* `tx.send(i)` returns `Result<(), SendError>` — fails if the receiver has been dropped.
* `rx.recv()` returns `Result<T, RecvError>` — fails when all senders are dropped and the channel is empty.
* `for received in rx` is sugar for "loop calling `rx.recv()` until it returns `Err`."
* `Sender` is `Clone` (multi-producer); `Receiver` is not (single-consumer).

For more sophisticated patterns (multi-consumer, bounded channels, select), the community crate `crossbeam-channel` is the standard. For async code, `tokio::sync::mpsc` is the equivalent.

## 3.7 A Worked Example: Concurrent Word Count

Let's tie this all together. Here's a program that counts words in several files concurrently.

```rust
use std::collections::HashMap;
use std::fs;
use std::sync::{Arc, Mutex};
use std::thread;

fn main() {
    let files = vec!["a.txt", "b.txt", "c.txt"];
    let counts = Arc::new(Mutex::new(HashMap::<String, usize>::new()));

    let handles: Vec<_> = files.into_iter().map(|path| {
        let counts = Arc::clone(&counts);
        thread::spawn(move || {
            // Read file. ? doesn't work in `main` so we use unwrap_or for demo.
            let contents = fs::read_to_string(path).unwrap_or_default();
            for word in contents.split_whitespace() {
                let mut map = counts.lock().unwrap();
                *map.entry(word.to_string()).or_insert(0) += 1;
                // map is dropped at end of this iteration; lock released.
            }
        })
    }).collect();

    for h in handles {
        h.join().unwrap();
    }

    let final_counts = counts.lock().unwrap();
    let mut pairs: Vec<_> = final_counts.iter().collect();
    pairs.sort_by_key(|&(_, c)| std::cmp::Reverse(*c));
    for (word, count) in pairs.iter().take(10) {
        println!("{:6} {}", count, word);
    }
}
```

What happens:

* Each thread takes one file path. Captures it via `move`.
* Each thread shares an `Arc<Mutex<HashMap>>` with the others.
* Inside the loop, each thread takes the lock, mutates the map, releases the lock.

This works correctly. It also has a **performance bug**: the lock is taken once per word, which is enormous contention. A better design would be: each thread builds its own local HashMap, and only at the end merges into the shared one. But the program is correct as-is. The point is you couldn't write a program with a *race* this way, only a slow one.

If you mistakenly wrote `Rc<RefCell<HashMap>>` instead, the compiler would refuse:

```
error[E0277]: `Rc<RefCell<HashMap<...>>>` cannot be sent between threads safely
```

Compile-time confidence.

## 3.8 Why Mutex Is Unfair, and Other Practicalities

A few things you'll learn the hard way if nobody warns you:

**Mutex is not reentrant.** If a thread already holds a `Mutex` and calls `.lock()` again on the same one, it deadlocks. Java's `synchronized` is reentrant; Rust's `Mutex` is not. Don't recursively lock.

**Lock order matters.** If thread A locks X then Y, and thread B locks Y then X, you have a classic deadlock. Establish a global lock order in your design and stick to it.

**Holding a lock while doing slow things is bad.** Don't lock, do I/O, then unlock. Lock, snapshot the state you need, unlock, then do I/O outside the lock.

**The fewer locks, the better.** A common refactoring: replace `Arc<Mutex<HashMap>>` with `Arc<HashMap<K, Mutex<V>>>` so that you only lock individual entries, not the whole map. Or use a concurrent map crate like `dashmap` that does this internally.

**`RwLock` for read-heavy workloads.** `std::sync::RwLock` allows many readers or one writer. Slightly more overhead per operation than `Mutex`, but enables genuine read concurrency. Reach for `RwLock` when reads dominate writes.

**Channels are usually clearer than locks.** "Pass data, don't share data" is the better design when feasible. A worker thread that owns its state and receives commands via a channel is much harder to break than a worker thread that mutates shared state under a lock.

## 3.9 Async: A Brief Orientation

Threads work, but each one costs about 8 MB of address space (the default stack) and a kernel object. For workloads with thousands of concurrent in-flight tasks (a web server handling 10,000 connections), threads run out of steam. Modern Rust uses **async/await** instead.

A *very* short tour. Async functions are declared with `async fn` and return a `Future` instead of executing immediately:

```rust
async fn fetch(url: &str) -> String {
    // ... some I/O ...
    String::from("contents")
}
```

Calling an async function does not run it; it returns a `Future`, which is a state machine that you have to `.await` (which makes the calling function also async) or hand to a runtime to drive.

The runtime — typically [Tokio](https://tokio.rs) — manages a small pool of OS threads (usually one per CPU core) and multiplexes thousands of futures onto them. When a future is waiting on I/O, the runtime parks it and moves on to other ready futures. This gives you Go-style "millions of concurrent things" performance with full Rust safety.

A minimal example:

```rust
use tokio::time::{sleep, Duration};

#[tokio::main]
async fn main() {
    let h1 = tokio::spawn(async {
        sleep(Duration::from_millis(100)).await;
        println!("first done");
    });
    let h2 = tokio::spawn(async {
        sleep(Duration::from_millis(50)).await;
        println!("second done");
    });

    h1.await.unwrap();
    h2.await.unwrap();
}
```

The output is `second done` then `first done`, after about 100 ms total — both tasks ran concurrently on the same thread.

Async/await is a big topic in its own right. The concepts to know for a 101:

* `async fn foo() -> T` returns a `Future<Output = T>`.
* `.await` is how you wait for a future, and only works inside an `async` context.
* You need a runtime to actually run anything. `tokio::main` macro is the easy way.
* Send + Sync still apply. `Arc<Mutex<T>>` still works. There's also `tokio::sync::Mutex` for cases where you need to hold a lock across an `.await` (which `std::sync::Mutex` doesn't support cleanly).
* The async ecosystem has its own versions of channels, file I/O, network I/O, etc. — all in Tokio.

For most server-side Rust written in 2026, async is the default. For CPU-bound work or simple programs, threads are still fine.

## 3.10 Summary: The Rules

1. **Single-thread shared ownership: `Rc<T>`.** Multiple owners, immutable. Reference counted, non-atomic, fast.
2. **Multi-thread shared ownership: `Arc<T>`.** Same idea, atomic counter, slightly slower.
3. **Single-thread interior mutability: `RefCell<T>`.** Mutate through shared ref. Borrow rules checked at runtime; panic on violation.
4. **Multi-thread interior mutability: `Mutex<T>` (or `RwLock<T>`).** Acquire lock to access. Released automatically on guard drop.
5. **The standard sharing pattern is `Arc<Mutex<T>>`.** Memorise it; you'll write it constantly.
6. **`Send` and `Sync` are auto-derived marker traits.** They prevent thread-unsafe types from crossing thread boundaries. The compiler enforces this.
7. **`Rc` is not `Send`. `RefCell` is not `Sync`.** Trying to send them across threads is a compile error.
8. **Use `thread::scope` for threads with bounded lifetime.** Avoids needing `Arc` for shared read-only data.
9. **Use `mpsc::channel` (or `crossbeam-channel`) to pass data between threads.** Often cleaner than locks.
10. **Locks are not reentrant. Establish lock order. Don't hold locks across slow operations.**
11. **For high-concurrency I/O, use async with Tokio.** For CPU-bound or simple programs, threads.

## 3.11 Drill 3

**Q1. The trait failure.**

The following code does not compile. Identify the error and fix it. Explain in your own words why the borrow checker rejected the original.

```rust
use std::rc::Rc;
use std::thread;

fn main() {
    let data = Rc::new(vec![1, 2, 3]);
    let data_clone = Rc::clone(&data);
    let handle = thread::spawn(move || {
        println!("{:?}", data_clone);
    });
    handle.join().unwrap();
}
```

In your fix, identify exactly which type changed and why the new version is safe.

**Q2. Build a thread-safe counter.**

Implement a struct `Counter` with these requirements:

* Multiple threads can hold a clone of the counter and call `.increment()` and `.value()`.
* Internally uses `Arc<Mutex<u64>>`.
* Provide a `clone` operation that increments the Arc count, not the integer.

Test it by spawning 100 threads that each increment 10,000 times. Final value must be 1,000,000.

Then: rewrite using `std::sync::atomic::AtomicU64` instead of `Mutex<u64>`. Benchmark both. Report ns per increment for each. Explain when you'd choose Mutex over Atomic, given that Atomic is faster.

**Q3. Channels for pipeline.**

Build a three-stage pipeline:

* Stage 1: a producer thread emits the integers 1 to 1,000,000.
* Stage 2: a worker thread consumes the integers, squares them, and forwards.
* Stage 3: the main thread consumes the squares and computes their sum.

Use `mpsc::channel` between stages. Time the whole pipeline. Then change the channels to `crossbeam-channel::bounded(1024)` and time again. Explain the difference (or lack thereof) in your benchmark numbers.

**Q4. The deadlock.**

The following code deadlocks. Predict whether it deadlocks every time, sometimes, or never, and explain. Then propose a fix.

```rust
use std::sync::{Arc, Mutex};
use std::thread;

fn main() {
    let a = Arc::new(Mutex::new(0));
    let b = Arc::new(Mutex::new(0));

    let a1 = Arc::clone(&a);
    let b1 = Arc::clone(&b);
    let h1 = thread::spawn(move || {
        let _x = a1.lock().unwrap();
        thread::sleep(std::time::Duration::from_millis(10));
        let _y = b1.lock().unwrap();
    });

    let a2 = Arc::clone(&a);
    let b2 = Arc::clone(&b);
    let h2 = thread::spawn(move || {
        let _x = b2.lock().unwrap();
        thread::sleep(std::time::Duration::from_millis(10));
        let _y = a2.lock().unwrap();
    });

    h1.join().unwrap();
    h2.join().unwrap();
}
```

After the fix, explain the rule you applied (it has a name).

**Q5. Async vs threads.**

Implement a function that fetches the contents of 10 URLs concurrently and returns a Vec of the results. Write it twice:

* Version A: using `std::thread::spawn` and `mpsc::channel` to collect results.
* Version B: using `tokio::spawn` and `futures::future::join_all`.

Use any HTTP client crate for the actual fetching (`reqwest` is the standard one; its blocking and async APIs are both available).

Then answer:

* If the URLs were 10,000 instead of 10, which version would scale better, and why?
* If each URL took 1 second to respond, what's the minimum total latency for each version?
* Would you ever choose Version A over Version B for an I/O-heavy program? Justify.

**Q6. Read the Rustonomicon entry on `Send` and `Sync`.**

The `Send` and `Sync` traits are defined and discussed in https://doc.rust-lang.org/nomicon/send-and-sync.html. Read it.

Then answer:

* Why is `Send` "almost always" auto-derived correctly, and when does the compiler get it wrong?
* What does it mean that "raw pointers (`*const T`, `*mut T`) are not Send"? Why is this safer than the C convention?
* The Nomicon describes `Send` and `Sync` as "the bedrock of Rust's concurrency story." After this drill, in your own words, why?

---

## Phase 1 Master Rules

A condensed reference, organised the way a senior Rust programmer would think about the language.

### Ownership and borrowing

* Every value has one owner. When the owner goes out of scope, the value is dropped.
* Assignment moves by default; the old binding becomes dead.
* `Copy` types duplicate; only cheap, owns-no-resources types are `Copy`.
* `&T` is a shared reference; many can coexist.
* `&mut T` is exclusive; while it exists, no other reference may.
* References can't outlive the data they point to. Lifetimes track this.
* Most lifetimes are inferred. Annotate when elision rules are insufficient.
* Prefer `&str` over `&String`, `&[T]` over `&Vec<T>` in function arguments.

### The type system

* Structs group named fields. Methods live in `impl` blocks.
* Enums are sum types. Pattern matching is exhaustive — the compiler will tell you about missed cases.
* `Option<T>` replaces null. `Result<T, E>` replaces exceptions.
* `?` propagates errors with automatic `From` conversion.
* Traits define behaviour, explicitly implemented per type.
* Derive `Debug`, `Clone`, `PartialEq` (and friends) on data types where they apply.
* Generics are zero-cost (monomorphised). Trait objects (`dyn`) are runtime-polymorphic and cost an indirect call.
* Custom error enums + `thiserror` is the production pattern for libraries. `anyhow` for binaries.
* Panic for bugs (assertion failures); `Result` for expected runtime conditions.

### Concurrency

* `Arc<T>` for multi-threaded shared ownership; `Rc<T>` for single-threaded.
* `Mutex<T>` for multi-threaded shared mutation; `RefCell<T>` for single-threaded.
* The default sharing primitive is `Arc<Mutex<T>>`. Memorise it.
* `Send` and `Sync` are auto-derived markers; the compiler enforces them at thread-boundary calls.
* `thread::scope` for short-lived threads that borrow from the parent.
* Channels (`mpsc`, `crossbeam-channel`) are often cleaner than locks.
* For I/O-heavy concurrency, async/await with Tokio. For CPU-bound or simple programs, threads.
* Locks are not reentrant. Establish lock order. Hold locks briefly.

### Tooling

* `cargo build` / `cargo run` / `cargo test` — the daily commands.
* `cargo clippy` — catches stylistic and semantic issues. Run regularly.
* `cargo fmt` — formats your code. Run before committing.
* `cargo doc --open` — generates HTML docs for your crate. The standard library docs work the same way.
* `rust-analyzer` — the language server. Install in your editor before doing anything else.

### Mindset

* If the borrow checker rejects your code, it's almost always telling you something true. Restructure rather than fight.
* Smaller functions = simpler borrow checker conversations.
* Reach for `clone()` and `unsafe` rarely. Both are escape hatches; using them everywhere defeats the point.
* Read the full compiler error message. Rust's diagnostics are excellent once you can decode them.
* The first month is the hard part. After that, the patterns become habits.

### Success criteria

If after Phase 1 you can:

* Read a struct or function signature and predict the ownership/borrowing it implies.
* Write a small program (a few hundred lines) using `Result` and `?` for error handling, with a custom error enum.
* Choose between `Rc`, `Arc`, `Box`, `Mutex`, `RefCell` correctly for a given sharing requirement.
* Spawn threads and pass data between them via channels or `Arc<Mutex<T>>`.
* Resolve common borrow checker errors without reaching for `.clone()` or `unsafe`.

Then you've got the foundation. Phase 2 (which we'll do another time) covers traits in depth (associated types, GATs, supertraits), the iterator-and-closure ecosystem, async runtimes deeper than the surface, macros, FFI, `unsafe` Rust properly, and the major application-domain crates (Tokio, Axum/Actix, Serde, Diesel/SQLx, Tonic).

---

*Phase 1 complete. Phase 2: traits in depth, iterators and closures, async, macros, unsafe, and the application-layer ecosystem.*
