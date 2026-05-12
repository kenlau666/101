# Rust — Phase 2 Course (Intermediate Edition)

> Idiomatic Rust, Async, and the Real-World Ecosystem.
> Three lessons. Same teaching style as Phase 1: gentle prose, harsh drills.
> **Assumes you completed Phase 1.** If you didn't, go back. None of this will make sense without ownership and the borrow checker in your bones.

---

## What Phase 2 Is For

You finished Phase 1. You can read and write basic Rust. You understand ownership, borrowing, lifetimes, enums, traits, `Result`, threads, `Arc<Mutex<T>>`. You can build a small program end to end.

Now read some real-world Rust. Open up the source of `tokio`, or `axum`, or `serde`, or `reqwest`. Within five minutes you will encounter:

* `impl Iterator<Item = T>` — what is `impl Trait`?
* `where T: Send + 'static` — what is the `'static` lifetime doing on a type?
* `async fn handle(&self) -> Result<Response, Error>` — async functions return what, exactly?
* `Box<dyn Fn(&str) -> Result<()> + Send + Sync + 'static>` — what *is* this monstrosity?
* `#[derive(Serialize, Deserialize)]` — how does this generate code?
* A function takes `impl Future<Output = T>`, calls `.await` on it, and you don't see any state machines anywhere.

These aren't exotic. They appear on the first page of every serious crate. Phase 1 didn't cover them because they require Phase 1 to be solid first. Now they're the next thing.

Phase 2 takes the things Phase 1 introduced lightly — traits, closures, async — and goes deep. It also covers macros (the things `#[derive(...)]` actually does), unsafe Rust (the small core that makes safe Rust possible), and the standard application-layer crates (Serde, Tokio, Axum, etc.) you'll meet on day one of any real Rust job.

The difficulty curve is different from Phase 1. Phase 1's difficulty came from a brand-new mental model (ownership). Phase 2's difficulty comes from depth and quantity — there's a lot of detail, and most of it is technical rather than conceptual. You won't fight the borrow checker the way you did in week one. You will, however, sit in front of a 200-line Tokio example and slowly trace through it, looking up types, until you can predict what each line does. That's a different kind of work.

Plan on 8–12 hours per lesson, with drills. Three weeks if you do one lesson per week and let things settle.

---

## A Phase 2 Glossary

* **Associated type** — a type that's part of a trait, named per impl. Different from a generic parameter; you can think of it as an output rather than an input.
* **Blanket impl** — an `impl SomeTrait for T` that applies to *every* `T` (possibly with bounds). The Rust standard library is full of these.
* **`impl Trait`** — anonymous-but-concrete trait-bound type. In return position: "this function returns *some* type that implements Trait." In argument position: same as a generic.
* **`dyn Trait`** — type-erased trait object. Runtime dispatch.
* **Object safety** — the property a trait must have to be usable as `dyn Trait`. Most traits are object-safe; some, like `Iterator` for some uses, are not.
* **Closure** — an anonymous function that may capture variables from its enclosing scope. Closures implement one of the `Fn`, `FnMut`, `FnOnce` traits.
* **`FnOnce`, `FnMut`, `Fn`** — the three closure traits, in order of decreasing flexibility. `FnOnce` can be called once; `FnMut` can be called many times and may mutate captured state; `Fn` can be called many times without mutation.
* **Iterator** — a trait with one method, `next() -> Option<Item>`. The basis of all looping in idiomatic Rust.
* **Lazy** — an iterator that doesn't do work until you ask for the next item. All Rust iterator adapters are lazy.
* **Future** — a value representing work that may complete later. A trait with one method, `poll`, that the runtime calls to make progress.
* **Runtime / executor** — a piece of code that runs futures. Tokio is the dominant runtime in production Rust.
* **Pin** — a wrapper that prevents a value from being moved in memory. Required for self-referential futures.
* **Macro** — code that writes code, expanded at compile time before the rest of compilation. Two flavours: `macro_rules!` (declarative) and procedural macros.
* **Procedural macro (proc-macro)** — a Rust function that takes a syntax tree as input and returns a syntax tree as output. The thing `#[derive(Serialize)]` uses.
* **Unsafe** — a keyword unlocking five specific super-powers (raw pointer deref, FFI, mutable static access, unsafe trait impl, unsafe fn call). Doesn't disable the borrow checker.
* **FFI** — Foreign Function Interface. Calling C from Rust, or letting C call Rust.
* **Serde** — the de-facto serialization framework. Handles JSON, YAML, MessagePack, Bincode, dozens of formats, all from one set of derives.
* **Tokio** — the de-facto async runtime. Plus a constellation of related crates (Hyper, Tower, Tonic, Axum, Tracing).

---

# Lesson 4: Traits, Iterators, and Closures in Depth

## 4.1 Why This Lesson Exists

A confession: idiomatic Rust looks weird at first. Open up real code and you see chains like:

```rust
let total: u64 = orders
    .iter()
    .filter(|o| o.status == Status::Filled)
    .map(|o| o.price * o.quantity as i64)
    .map(|notional| notional as u64)
    .sum();
```

Five method calls in a chain, no for-loop, no temporary variables. To a developer used to Java or Go, this is unsettling. Where's the loop? What's the type of each intermediate? Is this slow because of all the function calls?

The answer: this is the *fast* way. The compiler turns this chain into the same machine code as a hand-written for-loop. There are no intermediate allocations. Each closure inlines into the next. The whole pipeline is "fused" into a single tight loop. This is what people mean when they call iterators "zero-cost abstractions" — you write declarative code, the compiler emits imperative machine code, and you pay nothing for the abstraction.

To get this right, you need to understand three things deeply:

* **Traits beyond the basics** — associated types, blanket impls, `impl Trait`, the orphan rule. Without these, half of the standard library's docs read like nonsense.
* **Closures** — what they actually are, why there are three different `Fn*` traits, and when you can pass `impl Fn` versus when you need `Box<dyn Fn>`.
* **Iterators** — the trait, the adapters, the laziness, the rules for writing your own.

These three topics weave into each other. Iterators are a trait. Iterator adapters take closures as arguments. Closures implement traits. You can't fully grasp any one of them in isolation. Read this lesson straight through.

## 4.2 Traits, Beyond the Basics

Phase 1 introduced traits as "Rust's interfaces, but explicit." That's true and incomplete. Several features make traits considerably more powerful — and a few of them you'll see in every nontrivial codebase.

### Associated types

A trait can declare types as part of its contract, not just methods. The classic example is `Iterator`:

```rust
pub trait Iterator {
    type Item;
    fn next(&mut self) -> Option<Self::Item>;
    // ... many default methods omitted ...
}
```

`Item` is an **associated type**. Each implementor picks one concrete type for `Item`. A `Vec<i32>::iter()` returns an iterator with `Item = &i32`. A `String::chars()` returns one with `Item = char`. The trait says "you must produce *some* type, and you'll call it Item; you must implement next() to return Option of that type."

Why an associated type instead of a generic parameter? Compare:

```rust
// Associated type version (the real one):
trait IteratorA { type Item; fn next(&mut self) -> Option<Self::Item>; }

// Generic parameter version (hypothetical):
trait IteratorB<Item> { fn next(&mut self) -> Option<Item>; }
```

The difference is per-type cardinality. With `IteratorB<Item>`, a single struct could in principle implement `IteratorB<i32>` *and* `IteratorB<String>` and produce different things depending on context. With associated types, a struct implements `IteratorA` exactly once, with a single choice of `Item`. This matches the actual semantics of an iterator: a `Vec<i32>::Iter` produces `&i32`s, full stop, not "either `&i32` or `String` depending on what the caller wants."

The shorthand: **associated types are outputs, generic parameters are inputs.** When you call `next()`, you don't get to pick the item type — the iterator type determines it. So it's an output, so it's an associated type.

You'll see this pattern everywhere in the standard library:

* `Iterator::Item`
* `Future::Output`
* `Add::Output` (the type produced by `+`)
* `Deref::Target`

When you write your own trait, ask "if multiple implementors had different choices for this type, would it ever make sense for the same type to implement the trait in multiple ways?" If no (the usual case), use an associated type.

### Generic parameters versus associated types: bounds in functions

This affects how you write generic bounds. With an associated type, you constrain it like this:

```rust
fn sum<I>(iter: I) -> i64
where
    I: Iterator<Item = i64>,
{
    let mut total = 0;
    for x in iter {
        total += x;
    }
    total
}
```

The `Item = i64` syntax pins the associated type to a specific value. Any iterator producing `i64` works; iterators of `i32` or `String` don't.

You can also leave the associated type unconstrained:

```rust
fn count<I: Iterator>(iter: I) -> usize {
    let mut n = 0;
    for _ in iter { n += 1; }
    n
}
```

Here `Item` is unspecified — `count` works for any iterator regardless of the item type.

### Supertraits

A trait can require that implementors *also* implement another trait. This is called a **supertrait** relationship.

```rust
trait Animal {
    fn name(&self) -> &str;
}

// Pet requires Animal. Anything implementing Pet must also implement Animal.
trait Pet: Animal {
    fn owner(&self) -> &str;
}

struct Dog { name: String, owner: String }

impl Animal for Dog {
    fn name(&self) -> &str { &self.name }
}

impl Pet for Dog {
    fn owner(&self) -> &str { &self.owner }
}

fn introduce<P: Pet>(p: &P) {
    // Inside this function, P implements both Pet AND Animal.
    println!("{} belongs to {}", p.name(), p.owner());
    //                                ^-- from Animal, not Pet
}
```

You'll see this in the standard library: `Eq` requires `PartialEq`, `Ord` requires `PartialOrd + Eq`, `Copy` requires `Clone`. The supertrait says "you can't be `Eq` without first being `PartialEq`," which makes sense because total equality is a refinement of partial equality.

### Blanket implementations

A blanket impl is `impl<T> SomeTrait for T where T: ...`. It provides the trait for *every* type that meets the bounds. The standard library uses these heavily.

The most famous one:

```rust
impl<T: Display + ?Sized> ToString for T {
    fn to_string(&self) -> String { /* ... */ }
}
```

That single impl gives every `Display` type a `to_string()` method. You don't write `to_string` for your structs; the blanket impl provides it once your struct has `Display`. (The `?Sized` bound is an arcane detail meaning "don't require T to have a known compile-time size" — most generic bounds implicitly require Sized; opting out lets `str` and `[T]` participate.)

Another:

```rust
impl<T> From<T> for T {
    fn from(t: T) -> T { t }
}
```

Reflexive `From`: any type can be "converted from" itself. This is what makes `let x: String = some_string.into();` work when `some_string` is already a `String` — the blanket gives every type a no-op into.

Blanket impls are powerful and dangerous. If you wrote `impl<T> MyTrait for T` in your crate, you'd be claiming `MyTrait` for every type in the universe, including types from other crates and the standard library — and that's what the orphan rule prevents.

### The orphan rule

Here's the rule, exactly as it works:

> You can implement a trait for a type only if at least one of the trait or the type is defined in your own crate.

So: in *your* crate, you can write `impl Display for MyStruct` (your type, foreign trait — ok), or `impl MyTrait for Vec<i32>` (foreign type, your trait — ok), or `impl MyTrait for MyStruct` (both yours — obviously ok). What you *cannot* write is `impl Display for Vec<i32>` — both are foreign.

Why? Because if your crate could write that impl, and someone else's crate could write a different `impl Display for Vec<i32>`, the linker would have two contradictory implementations to choose from. There's no good answer. So the language rules out the situation entirely: only one crate is in a position to write any given impl.

This bites you regularly. You want to add a method to `Vec<T>`? You can't — `Vec` is foreign and the method is defined via traits. The workaround is the **newtype pattern**: wrap the foreign type in a struct of your own, and implement traits on the wrapper.

```rust
struct MyVec(Vec<i32>);

impl std::fmt::Display for MyVec {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        for x in &self.0 {
            write!(f, "{} ", x)?;
        }
        Ok(())
    }
}
```

Now `MyVec` is your type, so foreign traits like `Display` are fair game. The cost is wrapping/unwrapping at the boundary. Newtypes are also useful for making semantically distinct types — `struct UserId(u64)` and `struct OrderId(u64)` are different types even though both are 8 bytes, so you can't accidentally pass one where the other is expected.

### `impl Trait` in return position

Sometimes you want to return "something that implements a trait" without naming the concrete type. Maybe the type is unwieldy (like an iterator chain), or maybe you want freedom to change it later.

```rust
fn make_counter() -> impl Iterator<Item = u64> {
    (0u64..).map(|n| n * 2)
}
```

The actual return type here is `std::iter::Map<std::ops::RangeFrom<u64>, [closure]>` — verbose and tied to implementation details. `impl Iterator<Item = u64>` says "the caller can treat the return value as an iterator of u64 and otherwise should not care about the type." The compiler picks the concrete type; the caller just gets a thing that implements `Iterator`.

This is **not** dynamic dispatch. There's still one concrete type at compile time; the compiler inlines through it normally. The only constraint is the caller doesn't get to inspect the type.

A subtle restriction: a function returning `impl Trait` can return only one concrete type. This compiles:

```rust
fn make() -> impl Iterator<Item = u64> {
    (0u64..10).map(|x| x * 2)   // one concrete iterator type
}
```

This does not:

```rust
fn make(case: bool) -> impl Iterator<Item = u64> {
    if case {
        (0u64..10).map(|x| x * 2)              // type A
    } else {
        vec![1, 2, 3].into_iter()              // type B — different!
    }
}
```

The two branches return different concrete types, and `impl Trait` can hide *one* concrete type, not unify two. For "either of two types," you need `Box<dyn Iterator<Item = u64>>` or an enum.

### `impl Trait` in argument position

Same syntax, slightly different meaning:

```rust
fn print_all(iter: impl Iterator<Item = i64>) {
    for x in iter {
        println!("{}", x);
    }
}
```

In argument position, `impl Trait` is sugar for an anonymous generic parameter. The above is equivalent to:

```rust
fn print_all<I: Iterator<Item = i64>>(iter: I) {
    for x in iter {
        println!("{}", x);
    }
}
```

Same monomorphisation, same performance. The `impl Trait` form is shorter when you don't need to name the parameter elsewhere.

### Object safety, in passing

We covered `dyn Trait` in Phase 1 as runtime polymorphism. There's a wrinkle: not every trait can be made into a trait object. The compiler enforces a set of rules called **object safety**, and traits that violate them can't appear after `dyn`.

The two big rules:

* The trait can't have generic methods. (`fn foo<T>(&self, x: T)` makes the trait not object-safe.)
* The trait can't have methods that take `Self` by value or return `Self`. (`fn clone(&self) -> Self` is bad — but `fn clone(&self) -> Box<Self>` works.)

`Iterator` is object-safe; you can use `dyn Iterator<Item = i32>`. `Clone` is *not* object-safe (because `clone` returns `Self`); you can't have `dyn Clone`. The error message when you violate this is "the trait cannot be made into an object," which now you can decode.

Don't memorise the rules; remember that they exist, and when the compiler complains, look them up.

## 4.3 Closures

A **closure** is an anonymous function value, possibly capturing variables from its surrounding scope. The syntax:

```rust
let add_one = |x| x + 1;
let result = add_one(5);   // 6

let multiplier = 3;
let multiply = |x| x * multiplier;
//                  ^^^^^^^^^^
//                  captured from the surrounding scope
let result = multiply(5);   // 15
```

Closures look like a niche feature, but they're foundational because every iterator adapter takes one, and many higher-order functions in the ecosystem do too.

### Why are there three Fn traits?

```rust
trait FnOnce<Args> { type Output; fn call_once(self, args: Args) -> Self::Output; }
trait FnMut<Args>: FnOnce<Args> { fn call_mut(&mut self, args: Args) -> Self::Output; }
trait Fn<Args>: FnMut<Args> { fn call(&self, args: Args) -> Self::Output; }
```

(Simplified; the real signatures use unstable trait syntax. The relationships are exactly as shown: `Fn: FnMut: FnOnce`.)

A closure implements one or more of these depending on what it captures and how:

* **`Fn`**: takes `&self`. Can be called many times without mutating its captures. The most flexible.
* **`FnMut`**: takes `&mut self`. Can be called many times but mutates its captures, so callers need exclusive access while calling.
* **`FnOnce`**: takes `self` (consumes the closure). Can only be called once because calling it might consume captured variables.

The hierarchy: every `Fn` is also `FnMut` (read-only access is a subset of mutable access), and every `FnMut` is also `FnOnce` (a closure that can be called many times can certainly be called once). So if a function takes `FnOnce`, you can pass anything; if it takes `Fn`, the closure must avoid mutation.

Concretely:

```rust
let s = String::from("hello");

// Fn: borrows s immutably, can call many times.
let print = || println!("{}", s);
print();
print();

// FnMut: borrows s mutably, can call many times.
let mut s2 = String::from("hi");
let mut append = || s2.push_str("!");
append();
append();
println!("{}", s2);   // "hi!!"

// FnOnce: takes ownership of s3, can only call once.
let s3 = String::from("bye");
let consume = move || drop(s3);
consume();
// consume();   // error: closure already called
```

How does the compiler decide which trait a closure implements? By looking at how the closure body uses captures:

* Reads only → `Fn`.
* Mutates → `FnMut`.
* Consumes (e.g., `drop(s)` or moves the captured value into another function call by value) → `FnOnce`.

The `move` keyword changes the *capture mode*, not the trait. `move ||` says "take ownership of all captured variables when constructing the closure." But the closure can still implement `Fn` if its body only reads from those owned values:

```rust
let s = String::from("hello");
let print = move || println!("{}", s);
//          ^^^^ s is captured by move (the closure owns its own copy)
//                but the body only reads s, so the closure is still Fn.
print();
print();   // works
```

This matters for sending closures across threads. A closure passed to `thread::spawn` must be `'static` (no borrows from outside), so you typically write `move || { ... }` — but the closure can still be `Fn`.

### Function pointers vs closures

A regular function `fn` can be coerced into a closure type, and almost everywhere a closure is accepted, a function works:

```rust
fn double(x: i32) -> i32 { x * 2 }

let v: Vec<i32> = (1..=5).map(double).collect();      // function pointer
let v: Vec<i32> = (1..=5).map(|x| x * 2).collect();   // closure
```

If you don't capture anything, `|x| x * 2` is essentially a function pointer at runtime. Function pointers are 8 bytes on a 64-bit machine; non-capturing closures, by contrast, are *zero bytes*.

That's right — a closure with no captures has size zero. The compiler generates a unique anonymous type per closure, and if the closure has no captured state, the type has no fields, so it occupies no bytes. Calling it inlines to the function body.

### Returning closures

Returning a closure is a common requirement and a common confusion.

```rust
// "Add a constant to its argument."
fn make_adder(n: i32) -> impl Fn(i32) -> i32 {
    move |x| x + n
}

let add5 = make_adder(5);
println!("{}", add5(10));   // 15
```

`impl Fn(i32) -> i32` is the return type. Note `move`: without it, the closure would borrow `n`, which goes out of scope at the function's end, leaving a dangling reference. The `move` makes the closure take ownership of `n` (which is `Copy`, so the original `n` is also fine — but for non-Copy captures, the move is essential).

If you need to return one of several different closures (different sizes, different capture sets), `impl Trait` doesn't work because each branch is a different type. Then you reach for a boxed trait object:

```rust
fn make_op(plus: bool, n: i32) -> Box<dyn Fn(i32) -> i32> {
    if plus {
        Box::new(move |x| x + n)
    } else {
        Box::new(move |x| x - n)
    }
}
```

`Box<dyn Fn(i32) -> i32>` is a heap-allocated trait object. Calling it goes through a vtable lookup (one indirect call, ~5–10 ns). For most uses, this overhead is invisible; in tight loops, it can matter.

### `+ 'static`, `+ Send`, `+ Sync`

You'll often see closure-returning signatures with extra bounds:

```rust
fn make_handler() -> Box<dyn Fn(&Request) -> Response + Send + Sync + 'static> {
    // ...
}
```

`Send + Sync` means the closure can cross thread boundaries and be shared between threads. `'static` means the closure doesn't borrow anything from a shorter scope (so it can outlive the function that built it). These bounds aren't mysterious; they're the same `Send`, `Sync`, `'static` you already know, applied to the closure as a value.

## 4.4 Iterators

`Iterator` is the most-used trait in the standard library. It has one required method:

```rust
trait Iterator {
    type Item;
    fn next(&mut self) -> Option<Self::Item>;
    // Plus dozens of default methods built on top of next().
}
```

Every iterator promises: "ask me for `next()` and I'll either give you `Some(item)` or `None` once I'm done." That's it. Everything else — `map`, `filter`, `collect`, `sum`, `fold` — is built on top of that one method.

### Where iterators come from

Three kinds of iterators commonly come out of collections:

```rust
let v = vec![1, 2, 3];

// .iter()          → iterator of &T (borrows the collection)
for x in v.iter() {
    println!("{}", x);   // x: &i32
}

// .iter_mut()      → iterator of &mut T (mutably borrows the collection)
let mut v = vec![1, 2, 3];
for x in v.iter_mut() {
    *x *= 10;
}

// .into_iter()     → iterator of T (consumes the collection)
let v = vec![1, 2, 3];
for x in v.into_iter() {
    println!("{}", x);   // x: i32
}
// v can't be used here — into_iter consumed it.
```

A `for` loop without explicit method calls uses the default flavor, which depends on context. `for x in &v` is `iter()` (shared); `for x in &mut v` is `iter_mut()`; `for x in v` is `into_iter()` (consuming).

Many other things produce iterators: ranges (`0..10`), strings (`"abc".chars()`, `"abc".bytes()`, `"abc".split(' ')`), file readers, channel receivers, and so on.

### Laziness — the most important property

Iterator adapters are lazy. They don't do work until something consumes them.

```rust
let result = (0..10)
    .map(|x| {
        println!("mapping {}", x);
        x * 2
    });

// At this point, NOTHING has printed. We've built a lazy chain.

let collected: Vec<i32> = result.collect();
// NOW the prints happen, one per element, as collect drives the iterator.
```

This is a profound property. It means:

* You can build infinite iterators (`0..`) and only consume the first 100. Nothing infinite happens.
* Chains of `.map().filter().map()` don't allocate intermediate collections. Each item flows through the entire chain before the next item starts.
* You can compose generic operations into pipelines that compile down to a single tight loop.

The mental model: imagine each `.map()`, `.filter()`, etc. as wrapping the previous iterator in a new struct that, when polled with `next()`, calls `next()` on its inner iterator and transforms the result. The chain is a stack of structs, evaluated outside-in.

Functions that consume iterators (drive them to completion or partial completion) are called **consumers** or **terminators**: `collect`, `sum`, `count`, `for_each`, `fold`, `find`, `any`, `all`, `min`, `max`, `last`, `nth`, `reduce`. If your iterator chain never reaches one of these (or a `for` loop), it does nothing.

### The big four

You'll use these constantly. Memorise them.

**`.map(f)`**: transform each item. `Iterator<Item = A>` becomes `Iterator<Item = B>` where `f: A -> B`.

```rust
let doubled: Vec<i32> = (1..=5).map(|x| x * 2).collect();
// [2, 4, 6, 8, 10]
```

**`.filter(p)`**: keep items where `p` returns true. `p: &A -> bool`.

```rust
let evens: Vec<i32> = (1..=10).filter(|x| x % 2 == 0).collect();
// [2, 4, 6, 8, 10]
```

**`.collect()`**: drain the iterator into a collection. The target type is inferred from context, usually with a turbofish or annotation.

```rust
let v: Vec<i32> = (1..=5).collect();
let s: String = "hello".chars().rev().collect();
let m: HashMap<i32, i32> = (0..10).map(|i| (i, i*i)).collect();
```

`collect` is generic over the output type. You can collect into `Vec`, `String`, `HashMap`, `HashSet`, `BTreeMap`, `Result<Vec<_>, _>` (if the items are `Result`s, this short-circuits on the first error and returns it), and more.

**`.fold(init, f)`**: combine all items into a single value. `f: (Acc, A) -> Acc`.

```rust
let sum = (1..=10).fold(0, |acc, x| acc + x);   // 55
let product = (1..=5).fold(1, |acc, x| acc * x);   // 120
let concat = ["a", "b", "c"].iter().fold(String::new(), |mut acc, s| {
    acc.push_str(s);
    acc
});  // "abc"
```

`fold` is the most general consumer; almost every other terminator can be implemented on top of it. Use it when the specific terminator (`sum`, `count`, etc.) doesn't fit.

### The other adapters worth knowing

```rust
// .take(n) — take only the first n items
(0..).take(5).collect::<Vec<_>>();   // [0, 1, 2, 3, 4]

// .skip(n) — skip the first n items
(0..10).skip(7).collect::<Vec<_>>();   // [7, 8, 9]

// .chain(other) — concatenate two iterators
(0..3).chain(10..13).collect::<Vec<_>>();   // [0, 1, 2, 10, 11, 12]

// .zip(other) — pair items from two iterators
(0..3).zip(["a", "b", "c"]).collect::<Vec<_>>();
// [(0, "a"), (1, "b"), (2, "c")]

// .enumerate() — pair each item with its index
["a", "b", "c"].iter().enumerate().collect::<Vec<_>>();
// [(0, &"a"), (1, &"b"), (2, &"c")]

// .flat_map(f) — map each item to an iterator and flatten
vec!["abc", "de"].into_iter().flat_map(|s| s.chars()).collect::<String>();
// "abcde"

// .flatten() — flatten a nested iterator one level
vec![vec![1, 2], vec![3, 4]].into_iter().flatten().collect::<Vec<_>>();
// [1, 2, 3, 4]

// .find(p) — first item matching predicate, or None
(1..=10).find(|&x| x > 7);   // Some(8)

// .any(p), .all(p) — short-circuiting boolean checks
(0..100).any(|x| x == 50);   // true (stops at 50)
(0..10).all(|x| x < 100);    // true

// .count() — how many items
(0..1000).filter(|x| x % 7 == 0).count();

// .min(), .max() — Option<T>
[3, 1, 4, 1, 5, 9, 2, 6].iter().max();   // Some(&9)

// .sum(), .product() — total
(1..=10).sum::<i32>();   // 55
(1..=5).product::<i32>();   // 120
```

A common pattern: convert an iterator chain into the data structure you need.

```rust
let words: Vec<&str> = "the quick brown fox".split_whitespace().collect();
let pairs: Vec<(usize, &str)> = words.iter().enumerate().map(|(i, &w)| (i, w)).collect();
let lookup: HashMap<&str, usize> = words.iter().enumerate().map(|(i, &w)| (w, i)).collect();
```

### Iterators with `?`: collecting Results

A common situation: you have an iterator of `Result<T, E>` and want a `Result<Vec<T>, E>` — succeed with all values, or fail on the first error. `collect` handles this directly.

```rust
let inputs = vec!["1", "2", "not-a-number", "4"];
let parsed: Result<Vec<i32>, _> = inputs.iter().map(|s| s.parse::<i32>()).collect();
match parsed {
    Ok(v) => println!("{:?}", v),
    Err(e) => println!("failed: {}", e),
}
```

`collect::<Result<Vec<_>, _>>()` short-circuits on the first `Err` and returns it. This is one of those "clever" library tricks that, once you know about it, you reach for constantly.

### Writing your own iterator

Sometimes the standard adapters don't give you what you need, and you write a custom iterator. The recipe:

```rust
struct Fibonacci {
    curr: u64,
    next: u64,
}

impl Iterator for Fibonacci {
    type Item = u64;

    fn next(&mut self) -> Option<u64> {
        let result = self.curr;
        let new_next = self.curr + self.next;
        self.curr = self.next;
        self.next = new_next;
        Some(result)   // Fibonacci is infinite; we never return None.
    }
}

fn fib() -> Fibonacci {
    Fibonacci { curr: 0, next: 1 }
}

fn main() {
    let first_ten: Vec<u64> = fib().take(10).collect();
    println!("{:?}", first_ten);
    // [0, 1, 1, 2, 3, 5, 8, 13, 21, 34]
}
```

That's the whole pattern. Define a struct holding the iterator state, implement `Iterator` with a `next` method, return `Some(item)` while you have more or `None` when done. You inherit *all* the default methods (`map`, `filter`, `take`, etc.) for free.

### Iterators vs for loops: the question

A common question: should I write `iter.map(|x| ...).collect::<Vec<_>>()`, or a for loop?

Performance is identical in essentially all cases — the compiler fuses iterator chains into the same machine code as the equivalent loop. The decision is stylistic.

Iterators win when:

* The pipeline is naturally functional: filter, transform, collect.
* The chain has 2-4 stages and each is a one-liner.
* You're combining standard adapters (`zip`, `enumerate`, `take_while`).

For loops win when:

* The body has multiple statements with intertwined logic.
* You need early `return` or `break` from the enclosing function (closures can't do this without dancing).
* The control flow is genuinely imperative (state machines, complicated break conditions).
* Debugging — for loops set easier breakpoints than long iterator chains.

Idiomatic Rust uses iterators for the simple-pipeline case and for loops for the complicated-logic case. Don't be doctrinaire either way.

## 4.5 A Worked Example: A Tiny CSV Parser

Putting traits, closures, and iterators together. We'll write a function that parses a CSV-shaped string into structs:

```rust
#[derive(Debug)]
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
}

#[derive(Debug)]
enum ParseError {
    BadColumns(usize),   // saw this many columns; expected 3
    BadInt(String),
}

impl std::fmt::Display for ParseError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ParseError::BadColumns(n) => write!(f, "expected 3 columns, got {}", n),
            ParseError::BadInt(s) => write!(f, "couldn't parse {:?} as integer", s),
        }
    }
}

impl std::error::Error for ParseError {}

fn parse_int(s: &str) -> Result<i64, ParseError> {
    s.trim().parse::<i64>().map_err(|_| ParseError::BadInt(s.to_string()))
}

fn parse_orders(input: &str) -> Result<Vec<Order>, ParseError> {
    input
        .lines()                                       // Iterator<Item = &str>
        .filter(|line| !line.trim().is_empty())        // skip blanks
        .map(|line| {                                  // line -> Result<Order, ParseError>
            let cols: Vec<&str> = line.split(',').collect();
            if cols.len() != 3 {
                return Err(ParseError::BadColumns(cols.len()));
            }
            Ok(Order {
                id: parse_int(cols[0])? as u64,
                price: parse_int(cols[1])?,
                quantity: parse_int(cols[2])?,
            })
        })
        .collect()                                     // Result<Vec<Order>, ParseError>
}

fn main() {
    let csv = "
        1, 100, 5
        2, 200, 3

        3, 150, 7
    ";
    match parse_orders(csv) {
        Ok(orders) => {
            for o in &orders {
                println!("{:?}", o);
            }
        }
        Err(e) => println!("error: {}", e),
    }
}
```

Notes on what's at play:

* `input.lines()` gives an `Iterator<Item = &str>`.
* `.filter(...)` keeps a closure (an `Fn` because it only reads its argument).
* `.map(...)` keeps a closure that returns `Result<Order, ParseError>`.
* `.collect::<Result<Vec<Order>, ParseError>>()` (inferred from the function's return type) drains the iterator and short-circuits on the first error.
* `?` inside the closure works because the closure returns `Result`.

Twelve lines, no allocations except the final `Vec`, robust error handling. This is *what idiomatic Rust feels like*.

## 4.6 Summary: The Rules

1. **Associated types are outputs; generic parameters are inputs.** Use associated types when there's "one right answer per implementor."
2. **Supertraits express requirement: `trait B: A`** means every `B` is an `A`.
3. **The orphan rule:** you can implement a trait for a type only if at least one of them lives in your crate. The newtype pattern is the workaround.
4. **`impl Trait` in return position** hides one concrete type. In argument position, it's anonymous generic.
5. **Object safety** prevents some traits from being used as `dyn`. Generic methods and `Self`-by-value are the usual culprits.
6. **Three closure traits in increasing flexibility: `FnOnce`, `FnMut`, `Fn`.** The compiler picks based on capture usage.
7. **Closures with no captures are zero-sized.** Closures with captures might or might not need heap (depends on `Box<dyn ...>` vs `impl ...`).
8. **Iterators are lazy.** Adapters do nothing until a consumer (`collect`, `sum`, `for` loop, etc.) drives them.
9. **The big four iterator adapters: `map`, `filter`, `collect`, `fold`.** Plus `take`, `skip`, `chain`, `zip`, `enumerate`, `flat_map`, `find`, `any`, `all`.
10. **`collect::<Result<Vec<T>, E>>()`** short-circuits an iterator of Results. Use it.
11. **Iterator chains compile to the same machine code as for-loops.** Pick by clarity, not perf.

## 4.7 Drill 4

**Q1. Mechanism.**

In your own words, explain why `Iterator::Item` is an associated type instead of a generic parameter. What concrete bug or weirdness would arise if it were `trait Iterator<Item> { fn next(&mut self) -> Option<Item>; }` instead? Use a small code example to show the difference.

**Q2. The orphan rule.**

For each scenario, say whether the impl is allowed under the orphan rule, and if not, suggest the workaround.

* (a) In your crate `myapp`, you write `impl Display for Vec<MyStruct>`.
* (b) In your crate `myapp`, you write `impl MyTrait for i32`.
* (c) In your crate `myapp`, you write `impl Display for Vec<i32>`.
* (d) In your crate `myapp`, you write `impl Iterator for MyIter`.
* (e) In your crate `myapp`, you write `impl<T> MyTrait for Vec<T>`.

**Q3. Build a closure-taking function.**

Write a function with signature:

```rust
fn apply_n_times<F>(n: usize, f: F, initial: i64) -> i64
where
    F: Fn(i64) -> i64;
```

It applies `f` to `initial` `n` times in sequence and returns the result. So `apply_n_times(3, |x| x + 1, 10)` is `13`, and `apply_n_times(4, |x| x * 2, 1)` is `16`.

Now answer:

* Why does the bound say `F: Fn`, not `FnMut` or `FnOnce`? What changes if you switch to `FnMut`?
* Could the function take `impl Fn(i64) -> i64` instead of being generic over `F`? What's the difference, if any?
* Could the function take `Box<dyn Fn(i64) -> i64>`? What does that change?
* Write a benchmark comparing `impl Fn` and `Box<dyn Fn>` versions for a hot inner loop. Report the delta and explain it.

**Q4. Iterator surgery.**

Translate the following imperative code into idiomatic iterator-chain Rust. No for loops, no mutable accumulator variables outside the chain.

```rust
fn process(orders: &[Order]) -> Vec<(u64, i64)> {
    let mut result = Vec::new();
    for o in orders {
        if o.status == Status::Filled {
            let notional = o.price * o.quantity;
            if notional > 1000 {
                result.push((o.id, notional));
            }
        }
    }
    result.sort_by_key(|&(_, n)| std::cmp::Reverse(n));
    result.truncate(10);
    result
}
```

Then explain: which adapters did you use, and why? Did the iterator-chain version allocate any intermediate collections that the for-loop version didn't?

**Q5. Custom iterator.**

Implement a `Windows<T>` struct that iterates a slice in overlapping windows of a given size. So `Windows::new(&[1,2,3,4,5], 3)` yields `&[1,2,3]`, `&[2,3,4]`, `&[3,4,5]`, then `None`.

Requirements:

* Don't use the built-in `windows` method.
* `Windows<T>` should hold borrowed data — `&[T]` — not own a copy.
* The iterator's `Item` should be `&[T]`, with appropriate lifetime.

Hint: lifetimes are the hard part. You'll write `impl<'a, T> Iterator for Windows<'a, T>`.

After implementing, write a test that exercises it. Then chain `.map()` and `.collect()` on the iterator to confirm it composes correctly with standard adapters.

**Q6. The lazy proof.**

Write a program that demonstrates iterator laziness empirically. Build a chain like `(0..).map(|x| { println!("touched {}", x); x * 2 }).filter(|&x| x > 10).take(3).collect::<Vec<_>>()`. Predict in advance how many `touched` prints you'll see, and which numbers, and why. Then run it and verify your prediction.

Bonus: change `.take(3)` to `.skip(100).take(3)` and re-predict.

**Q7. Reading.**

Read the official iterator chapter: https://doc.rust-lang.org/std/iter/index.html (the module-level docs).

Also browse the iterator method list. There are many adapters you didn't see in this lesson (`peekable`, `cycle`, `dedup`, `step_by`, `partition`, `unzip`, `scan`). Pick three you didn't know, read their docs, and write a one-paragraph explanation of each in your own words with a tiny example.

---

# Lesson 5: Async Rust

## 5.1 Why This Lesson Exists

In 1999, a system administrator named Dan Kegel wrote an article called "The C10K Problem." The problem he described: how do you build a web server that handles 10,000 concurrent connections? The standard approach at the time was "one thread per connection," and at 10,000 connections you ran out of memory (each thread needed several MB of stack), out of CPU (the kernel scheduler couldn't switch fast enough), and out of file descriptors. The article argued that the future was *non-blocking I/O* — a small number of threads, each multiplexing many connections via OS primitives like `epoll` and `kqueue`.

Kegel was right. Every modern high-throughput server — Nginx, Node.js, Netty, Vert.x, Go's net/http, Tokio — is built on non-blocking I/O. The differences are in how the language exposes it.

* **Node.js / JavaScript:** callbacks, then promises, then async/await. The runtime is the event loop.
* **Go:** goroutines and channels. Blocking I/O calls are intercepted by the runtime, which transparently parks the goroutine and schedules another. Looks synchronous, behaves async.
* **Java (modern):** Project Loom virtual threads (since Java 21) — similar to Go.
* **Python:** asyncio, with explicit `async/await`.
* **Rust:** explicit `async/await`, no built-in runtime — you choose one (Tokio, async-std, smol).

Rust's design has two unusual properties. First, async is **zero-overhead**. A future doesn't cost a thread; it doesn't cost an allocation in many cases; it compiles to a state machine that's about as efficient as hand-rolled state-machine code. Second, async is **decoupled from the runtime**. The standard library defines what a Future is, but doesn't ship a runtime to drive them — you depend on a separate crate. In practice, that crate is almost always Tokio.

This decoupling makes async Rust harder to learn than async in other languages. You're not learning one thing (async syntax); you're learning two (the language feature, and the runtime). And the underlying mechanism — the Future trait, polling, Pin — is *exposed*, not hidden, so when something goes wrong you can be looking at compilation errors that mention `Pin<&mut Self>` and `Poll::Pending` and not understand a word of it.

This lesson takes async from the bottom up: what a Future actually is, how `async fn` desugars, what a runtime does, the standard patterns you'll use daily, and the standard pitfalls you'll hit weekly. The goal isn't to make you an async expert — that takes a year. The goal is to make you fluent enough that you can read a Tokio program, understand what each line is doing, and write your own without panic.

## 5.2 What is a Future, mechanically

Skip this if you just want to use async. Read it if you want to ever debug async problems past the surface level.

In Rust, an async value is anything implementing the `Future` trait:

```rust
trait Future {
    type Output;
    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output>;
}

enum Poll<T> {
    Ready(T),
    Pending,
}
```

That's the whole trait. One method, `poll`. The runtime calls `poll` on a future to make progress; the future returns either `Ready(value)` if it's done or `Pending` if it can't progress yet.

Think of a future as a state machine that the runtime advances by calling `poll`. Each call, the future does as much work as it can, then either finishes (`Ready`) or hits something blocking and returns `Pending`. When the future returns `Pending`, it has registered a callback (a "waker") with whatever it's waiting on (a socket, a timer, a channel). When that thing becomes ready, the waker fires and tells the runtime "this future can make progress now"; the runtime polls it again.

It's a pull-based model. The runtime is in charge — it polls when it wants. The future is passive; it computes during a poll, registers wakers, and waits.

### What `async fn` desugars to

You write:

```rust
async fn fetch(url: &str) -> Result<String, Error> {
    let response = http_get(url).await?;
    let body = response.body().await?;
    Ok(body)
}
```

The compiler generates something morally like (the real generated code is more complex):

```rust
fn fetch(url: &str) -> impl Future<Output = Result<String, Error>> {
    enum FetchState {
        Initial,
        WaitingForResponse(/* state to resume http_get */),
        WaitingForBody(/* state to resume body reading */),
        Done,
    }

    struct FetchFuture<'a> {
        state: FetchState,
        url: &'a str,
        // ... captured locals ...
    }

    impl<'a> Future for FetchFuture<'a> {
        type Output = Result<String, Error>;
        fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
            // big match on self.state, advancing each .await as poll calls
            // come in, transitioning between states, returning Pending where
            // the underlying future returned Pending, returning Ready when done.
        }
    }

    FetchFuture { state: FetchState::Initial, url, /* ... */ }
}
```

Key observations:

* Each `.await` is a state-machine boundary. The compiler splits the function at every `.await` and generates a state for each piece.
* Local variables that are alive across `.await` boundaries are stored as fields on the future struct (so they survive across polls).
* Local variables only used between two `.await`s, not across one, can stay on the stack of a single `poll` call.
* The future struct is named anonymously by the compiler; you reference it via `impl Future` at the function signature.

This is the *zero-cost* part. There's no thread per future, no heap allocation per await, no garbage collector. The future is a compact struct containing exactly the state it needs. Many of them fit in cache; the runtime's job is just to poll them in some order.

### The Pin and self-references

Here's where things get fiddly. The future struct above has fields like `url: &'a str`. That's a reference to data outside the future. But what if the future has a reference to its *own* data? Consider:

```rust
async fn example() -> u8 {
    let buf = vec![1u8, 2, 3];
    let r = &buf[0];   // borrow into buf, alive across the await below
    some_async_fn().await;
    *r
}
```

The compiler stores both `buf` and `r` as fields of the generated future struct. `r` is a pointer into `buf`. Both fields live in the same struct. **The future is self-referential.**

If the runtime moves the future from one memory location to another (which it might, when scheduling), then `buf`'s bytes move, but `r`'s pointer value doesn't update — `r` now points to where `buf` *used* to be, which is freed memory. Use-after-free.

`Pin` exists to prevent this. A `Pin<&mut T>` is a "promise that this `T` will not be moved" wrapper. The `Future::poll` method takes `Pin<&mut Self>`, which is the runtime's commitment that it won't move the future between polls.

In day-to-day code you almost never see `Pin` directly. The compiler hides it. You see it when:

* A compile error mentions Pin, usually because you tried to put a future in a place it can't go (e.g., a non-pinned `&mut`).
* You're writing a custom `Future` impl by hand (rare in application code).
* You're holding a future across an `.await` and the borrow checker complains.

For Phase 2 learning purposes: know that Pin exists, know it's about preventing moves, and don't be scared when you see it.

## 5.3 You Need a Runtime

The standard library defines `Future` and provides `async/await` syntax, but does *not* run futures. To actually execute a future, you need a runtime. The runtime is responsible for:

* Polling futures.
* Tracking which futures are "ready" to poll versus "waiting" on something.
* Managing the wakeup mechanism (epoll/kqueue/IOCP under the hood).
* Providing async-friendly primitives (timers, network sockets, channels, mutexes).

In production Rust, the runtime is **Tokio** in maybe 90% of cases. Other options exist (async-std, smol, embassy for embedded), but if you don't have a strong reason to differ, use Tokio.

The minimal Tokio program:

```rust
#[tokio::main]
async fn main() {
    println!("hello, async");
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
    println!("done");
}
```

`#[tokio::main]` is a macro that expands to roughly:

```rust
fn main() {
    let runtime = tokio::runtime::Runtime::new().unwrap();
    runtime.block_on(async {
        println!("hello, async");
        tokio::time::sleep(std::time::Duration::from_millis(100)).await;
        println!("done");
    });
}
```

It builds a runtime, hands it the async block as the "root future," and tells it to drive that future to completion. By default the runtime uses one OS thread per CPU core (a multi-threaded executor). For single-threaded behaviour, use `#[tokio::main(flavor = "current_thread")]`.

`Cargo.toml`:

```toml
[dependencies]
tokio = { version = "1", features = ["full"] }
```

The `full` feature turns on every Tokio sub-feature. For production you'd cherry-pick what you need.

### `block_on`, `spawn`, `join`

The three common ways to run futures:

* **`runtime.block_on(future)`** — synchronously wait for a future to complete. Used in `main` (or wherever you cross from sync to async). The current thread parks until the future finishes.
* **`tokio::spawn(future)`** — submit a future to the runtime to run *concurrently* with the caller. Returns a `JoinHandle`. Conceptually like spawning a goroutine in Go.
* **`future1.await`** — wait for one future from inside another. The current task pauses; the runtime schedules other ready tasks meanwhile.

Putting them together:

```rust
#[tokio::main]
async fn main() {
    // Spawn three concurrent tasks.
    let h1 = tokio::spawn(work(1, 100));
    let h2 = tokio::spawn(work(2, 50));
    let h3 = tokio::spawn(work(3, 200));

    // Wait for all of them. The order is by completion, so h2 (50ms) finishes first.
    let r1 = h1.await.unwrap();
    let r2 = h2.await.unwrap();
    let r3 = h3.await.unwrap();

    println!("{} {} {}", r1, r2, r3);
}

async fn work(id: u64, ms: u64) -> u64 {
    tokio::time::sleep(std::time::Duration::from_millis(ms)).await;
    id * 10
}
```

All three tasks run concurrently. Total wall-clock time: about 200ms (the slowest task). If you wrote this synchronously with three `sleep`s in a row, it'd take 350ms.

### Tasks and futures: not quite the same

A future is a value. A task is a future submitted to the runtime to run independently. `tokio::spawn(future)` creates a task. The runtime owns the task and polls it as needed. `future.await` (without spawn) doesn't create a new task; it embeds the future into the calling task's state machine.

This distinction matters because:

* Tasks can run in parallel on different threads (in a multi-threaded runtime).
* Futures inside a single task run *sequentially* (only one part of the state machine progresses at a time).
* Tasks have more overhead per item (a separate allocation for the runtime to track).

If you want concurrency, you spawn. If you just want sequential async work in one task, you don't.

## 5.4 The Daily Patterns

These are the patterns you'll write 100 times a week.

### Concurrent fan-out: `join`

When you have a fixed set of futures and want all of them:

```rust
use tokio::join;

async fn fetch_all() -> (Result<String, Error>, Result<String, Error>) {
    let f1 = fetch("https://a.example.com");
    let f2 = fetch("https://b.example.com");
    let (r1, r2) = join!(f1, f2);
    (r1, r2)
}
```

`join!` polls both futures concurrently until both finish, then gives you both results. Note: `f1` and `f2` are futures, not running tasks. The concurrency happens because `join!` polls both — if one is `Pending`, the other gets a chance.

For an arbitrary number of futures, `futures::future::join_all`:

```rust
use futures::future::join_all;

async fn fetch_n(urls: Vec<&str>) -> Vec<Result<String, Error>> {
    let futures: Vec<_> = urls.into_iter().map(fetch).collect();
    join_all(futures).await
}
```

### Concurrent fan-out with bounded parallelism: `buffer_unordered`

`join_all` has no concurrency limit — give it 10,000 futures, all 10,000 try to make progress at once. Often you want to cap the in-flight count, especially for I/O. Use stream-based processing:

```rust
use futures::stream::{self, StreamExt};

async fn fetch_capped(urls: Vec<&str>) -> Vec<Result<String, Error>> {
    stream::iter(urls)
        .map(|url| fetch(url))
        .buffer_unordered(8)    // at most 8 in flight at a time
        .collect()
        .await
}
```

`buffer_unordered(8)` keeps 8 futures running; as one finishes, it pulls the next from upstream. You almost never want unlimited concurrency in production — you DDoS your own dependencies. Always cap.

### Race: `select!`

`select!` polls multiple futures and returns whichever finishes first.

```rust
use tokio::select;
use tokio::time::{sleep, Duration};

async fn with_timeout() {
    select! {
        result = some_long_computation() => {
            println!("computed: {:?}", result);
        }
        _ = sleep(Duration::from_secs(5)) => {
            println!("timed out");
        }
    }
}
```

When one branch fires, the others are dropped. Dropping a future is how Rust expresses cancellation — see below.

For the common timeout case, there's a one-liner:

```rust
use tokio::time::{timeout, Duration};

let result = timeout(Duration::from_secs(5), some_long_computation()).await;
// result: Result<T, Elapsed>
```

### Channels

Tokio provides several channel types:

* **`tokio::sync::mpsc`** — multi-producer, single-consumer, bounded. The default for "send work to a worker."
* **`tokio::sync::oneshot`** — single message, single producer, single consumer. The default for "request/response between two tasks."
* **`tokio::sync::broadcast`** — multi-producer, multi-consumer, all consumers see every message. Like a fan-out queue.
* **`tokio::sync::watch`** — single-producer, multi-consumer, consumers see only the latest value. Like a config-broadcast.

```rust
use tokio::sync::mpsc;

async fn worker(mut rx: mpsc::Receiver<i32>) {
    while let Some(msg) = rx.recv().await {
        println!("got {}", msg);
    }
}

#[tokio::main]
async fn main() {
    let (tx, rx) = mpsc::channel(32);   // bounded, capacity 32

    tokio::spawn(worker(rx));

    for i in 0..10 {
        tx.send(i).await.unwrap();
    }
    drop(tx);   // close channel; worker exits its loop
}
```

`tx.send(i).await` blocks (asynchronously!) when the channel is full. This gives you natural backpressure — fast producers slow down to match slow consumers.

### Holding state across tasks

Same patterns as Phase 1's threads, but tweaked for async:

```rust
use std::sync::Arc;
use tokio::sync::Mutex;

#[derive(Default)]
struct State {
    count: u64,
}

#[tokio::main]
async fn main() {
    let state = Arc::new(Mutex::new(State::default()));

    let mut handles = Vec::new();
    for _ in 0..10 {
        let state = Arc::clone(&state);
        handles.push(tokio::spawn(async move {
            let mut s = state.lock().await;
            s.count += 1;
        }));
    }

    for h in handles {
        h.await.unwrap();
    }

    println!("{}", state.lock().await.count);
}
```

Note: this uses **`tokio::sync::Mutex`**, not `std::sync::Mutex`. Why? Because `tokio::sync::Mutex::lock()` returns a future — when the lock is held by someone else, your task yields and waits asynchronously. `std::sync::Mutex` blocks the OS thread, which in async land would block other tasks scheduled on the same thread.

The rule: if you hold the lock across an `.await`, use `tokio::sync::Mutex`. If you don't (acquire, mutate quickly, release, all sync), `std::sync::Mutex` is fine and faster. Most code uses `std::sync::Mutex` for short critical sections and `tokio::sync::Mutex` only when needed.

## 5.5 The Pitfalls

This is where most async bugs come from. Read carefully.

### Holding non-Send state across .await

In a multi-threaded runtime, a task can be polled on different threads at different times (the runtime moves it around for load balancing). For that to be safe, the *future itself* must be `Send`. Which means everything held across an `.await` boundary must be `Send`.

```rust
use std::rc::Rc;

async fn buggy() {
    let data = Rc::new(vec![1, 2, 3]);   // Rc is !Send
    some_async_fn().await;                 // <-- data is held across this await
    println!("{:?}", data);
}

tokio::spawn(buggy());   // ERROR: future is not Send
```

The fix: use `Arc` instead of `Rc`. The compiler's error message will tell you exactly which value isn't `Send`.

### Holding sync Mutex across .await

A subtle one. `std::sync::MutexGuard` is `!Send` (intentionally — releasing a mutex on a different thread than acquired it would be a bug). So if you hold a guard across an `.await`, the future is `!Send` and the multi-threaded runtime refuses to spawn it.

```rust
use std::sync::Mutex;

async fn bad(m: &Mutex<i32>) {
    let mut g = m.lock().unwrap();      // guard is held...
    *g = 5;
    some_async_fn().await;              // ...across this await. ERROR.
}

async fn good(m: &Mutex<i32>) {
    {
        let mut g = m.lock().unwrap();
        *g = 5;
    }   // guard dropped here
    some_async_fn().await;              // OK, no guard held now
}
```

If you genuinely need to hold a lock across awaits — typically because the async work *depends* on the locked state — use `tokio::sync::Mutex`, whose guard is `Send`.

### Blocking the executor

A future is supposed to be cooperative: it does work, hits an await, yields. If a future does heavy CPU work without yielding, it blocks the runtime thread it's on, which prevents other tasks scheduled on that thread from running.

```rust
async fn bad() {
    let huge_sum: u64 = (0..100_000_000).sum();   // CPU-bound, takes seconds
    println!("{}", huge_sum);
}
```

If you `tokio::spawn(bad())`, that task hogs its executor thread for the duration. In a multi-threaded runtime with N threads, you've lost 1/N of your throughput. In a single-threaded runtime, you've stalled *everything*.

The fix: `tokio::task::spawn_blocking`. This sends a closure to a separate thread pool dedicated to blocking work, leaving the async executor free.

```rust
async fn good() -> u64 {
    let huge_sum = tokio::task::spawn_blocking(|| {
        (0..100_000_000u64).sum::<u64>()
    }).await.unwrap();
    println!("{}", huge_sum);
    huge_sum
}
```

Use `spawn_blocking` for: heavy CPU work, calls to non-async libraries that block (file I/O via `std::fs`, blocking database drivers), legacy code you can't make async.

A heuristic: if a function call could take more than 100 microseconds and it's not async, it shouldn't run on the async executor. Move it to `spawn_blocking` or a dedicated thread.

### Cancellation by drop

When a future is dropped before it completes, all its in-flight state is discarded. This is the language's only cancellation mechanism — there's no `cancel()` method on futures.

```rust
use tokio::time::{timeout, Duration};

async fn process_message(msg: Message) {
    let result = timeout(Duration::from_secs(5), do_work(&msg)).await;
    match result {
        Ok(Ok(_)) => { /* success */ }
        Ok(Err(_)) => { /* do_work returned Err */ }
        Err(_) => {
            // do_work was dropped (cancelled) at the timeout boundary.
            // Whatever state it had is gone.
        }
    }
}
```

Two consequences:

* **Cancellation can happen at every `.await` point.** Your async function might run, hit an `.await`, never return, and be dropped. Local cleanup happens via `Drop`. Anything you started that should always finish (sending a "done" message, releasing an external resource) needs to either be in the futures it transitively awaits, or wrapped in a guard whose `Drop` impl handles cleanup.
* **Operations that *must not* be cancelled need to be spawned, not awaited.** `tokio::spawn(important_work())` runs `important_work` to completion regardless of what happens to the spawner. The spawner gets a `JoinHandle` it can either `.await` or drop; dropping the handle does *not* cancel the spawned task. (Note: `JoinHandle::abort()` does cancel it. Just dropping doesn't.)

This is a frequent source of subtle bugs. "I called `do_work().await` inside a `select!` and the timeout fired and now the database is in a weird state because the transaction was mid-commit when we cancelled." The lesson: be aware of which awaits can be cancelled, and design accordingly.

### Spawning futures that don't run

This compiles and does nothing:

```rust
async fn doesnt_run() {
    let _f = some_async_fn();   // builds the future, doesn't await it
    println!("hi");
}
```

`some_async_fn()` returns a future. Without `.await` or `spawn`, the future is built and immediately dropped. None of its body executes. This is laziness — same as iterators not running until consumed.

The "I called the function but it didn't seem to do anything" bug comes from forgetting `.await` or `spawn`. The compiler usually warns ("unused implementer of Future"). Don't ignore that warning.

## 5.6 A Worked Example: A Concurrent Fetcher with Backpressure

Let's tie everything together. We'll write a program that fetches a list of URLs with capped concurrency, prints results as they arrive, and times out individual requests.

```rust
use std::time::Duration;
use futures::stream::{self, StreamExt};
use tokio::time::timeout;

#[derive(Debug)]
struct FetchResult {
    url: String,
    status: Option<u16>,
    bytes: Option<usize>,
    error: Option<String>,
}

async fn fetch_one(client: &reqwest::Client, url: String) -> FetchResult {
    let result = timeout(Duration::from_secs(5), client.get(&url).send()).await;
    match result {
        Ok(Ok(resp)) => {
            let status = resp.status().as_u16();
            match resp.bytes().await {
                Ok(bytes) => FetchResult {
                    url,
                    status: Some(status),
                    bytes: Some(bytes.len()),
                    error: None,
                },
                Err(e) => FetchResult {
                    url,
                    status: Some(status),
                    bytes: None,
                    error: Some(format!("body read: {}", e)),
                },
            }
        }
        Ok(Err(e)) => FetchResult {
            url,
            status: None,
            bytes: None,
            error: Some(format!("request failed: {}", e)),
        },
        Err(_) => FetchResult {
            url,
            status: None,
            bytes: None,
            error: Some("timed out".to_string()),
        },
    }
}

#[tokio::main]
async fn main() {
    let urls = vec![
        "https://example.com",
        "https://www.rust-lang.org",
        "https://crates.io",
        // ... up to thousands ...
    ];

    let client = reqwest::Client::new();

    stream::iter(urls)
        .map(|url| fetch_one(&client, url.to_string()))
        .buffer_unordered(16)   // 16 in flight at once
        .for_each(|result| async move {
            println!("{:?}", result);
        })
        .await;
}
```

What's happening:

* `stream::iter(urls)` produces a stream of strings (a stream is the async equivalent of an iterator).
* `.map(...)` turns each string into a future.
* `.buffer_unordered(16)` polls up to 16 futures concurrently, yielding results as they finish.
* `.for_each(...)` consumes the stream, printing each result.
* `&client` is borrowed by each future; reqwest's client is designed to be cheap to share, internally pooling connections.

Read this against the "C10K problem" framing. With one thread per fetch and synchronous I/O, this design would consume 16 thread stacks (~128 MB) and shuffle data through the kernel scheduler. Async Rust runs all 16 concurrent fetches on a single OS thread (or a small handful in a multi-thread runtime), with kilobytes of state per fetch. It scales to thousands of concurrent fetches without sweating.

## 5.7 Streams: Iterators for Async

Streams are to futures as iterators are to plain values. The trait:

```rust
trait Stream {
    type Item;
    fn poll_next(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>>;
}
```

Compare to Iterator:

```rust
trait Iterator {
    type Item;
    fn next(&mut self) -> Option<Self::Item>;
}
```

The shape is identical; `poll_next` is just the async version of `next`. Streams have iterator-like adapters: `map`, `filter`, `take`, `chain`, etc. They live in the `futures` and `tokio_stream` crates.

You'll encounter streams when:

* Reading from a TCP socket (chunks of bytes arrive over time).
* Subscribing to events (new orders on an exchange, new rows in a database).
* Iterating with bounded concurrency (`buffer_unordered`, as above).

If you're comfortable with iterators (Lesson 4), streams are a small extension.

## 5.8 Async Trait Methods

A wart of the language: putting `async fn` in a trait was unstable for years and only stabilised relatively recently (and even now has limitations). You'll encounter two patterns:

**Modern (1.75+):**

```rust
trait DataStore {
    async fn get(&self, key: &str) -> Option<String>;
}
```

This works for traits used directly as generics (`fn use<D: DataStore>(d: &D)`). It does *not* work nicely for `dyn DataStore` — you need workarounds.

**With the `async-trait` crate:**

```rust
use async_trait::async_trait;

#[async_trait]
trait DataStore: Send + Sync {
    async fn get(&self, key: &str) -> Option<String>;
}
```

The macro rewrites each `async fn` to return `Pin<Box<dyn Future + Send>>`, allocating the future on the heap. Slower, but works seamlessly with `dyn DataStore`.

The community is gradually moving toward native async traits, but most existing crates still use `async-trait`. Both patterns are common; you'll encounter both.

## 5.9 Summary: The Rules

1. **A future is a state machine the runtime polls.** Each `.await` is a state-machine boundary.
2. **Async is zero-overhead.** No thread per future, often no allocation, no GC.
3. **Async is decoupled from the runtime.** You need to choose a runtime; Tokio is the default.
4. **`tokio::spawn(future)` runs concurrently. `future.await` runs sequentially within a task.**
5. **Use `join!` or `join_all` for parallel "all of these."** Use `select!` for "first of these." Use `buffer_unordered(N)` for "fan out with concurrency cap N."
6. **Don't hold non-Send state across `.await` if the task might cross threads.** That includes `Rc` and `std::sync::MutexGuard`.
7. **Don't block the executor.** Use `spawn_blocking` for CPU-heavy work or non-async I/O.
8. **Cancellation happens by drop.** Every `.await` is a potential cancellation point. Plan for it.
9. **Futures don't run until you `.await` or `spawn` them.** Forgetting this is a common source of "did nothing" bugs.
10. **`tokio::sync::Mutex` for locks held across awaits; `std::sync::Mutex` for short critical sections.**
11. **Streams are async iterators. Same shape, same adapters.**

## 5.10 Drill 5

**Q1. Mechanism — explain what `async fn` desugars to.**

In your own words and using a small concrete example, explain the state machine the compiler generates for an async function with two `.await` points. Mention: the enum of states, where local variables live, what each `poll` call does, and what happens when a child future returns `Pending`.

You should be able to write 200+ words. This is the foundational mental model; if you can't explain it, async will keep mystifying you.

**Q2. Run a real fetcher.**

Implement the URL fetcher from section 5.6 in full, with these requirements:

* Reads URLs from a file `urls.txt`, one per line.
* Caps in-flight requests at 32.
* Per-request timeout: 5 seconds.
* Total runtime budget: 30 seconds (after which the program prints whatever it has and exits).
* Outputs one line per result: `<url> <status> <bytes>` or `<url> ERROR: <reason>`.

Use `reqwest` for HTTP, `tokio` for async, `futures` for streams. Run it on a list of 100 URLs (any 100 — Wikipedia's "Special:Random" is convenient). Confirm that runtime is approximately the slowest 4 requests, not the sum.

Then add a metric: print the p50, p95, and max latency at the end. Use `hdrhistogram` or just collect into a vector and compute.

**Q3. Find and explain the bug.**

The following code compiles and seems to work, but contains an async bug that will manifest at high load. Identify it, explain the mechanism, and propose a fix.

```rust
use std::sync::{Arc, Mutex};
use tokio::time::{sleep, Duration};

async fn handle_request(state: Arc<Mutex<u64>>) {
    let mut count = state.lock().unwrap();
    *count += 1;
    sleep(Duration::from_millis(50)).await;   // simulate I/O
    println!("{}", *count);
}

#[tokio::main]
async fn main() {
    let state = Arc::new(Mutex::new(0u64));
    let mut handles = Vec::new();
    for _ in 0..100 {
        handles.push(tokio::spawn(handle_request(Arc::clone(&state))));
    }
    for h in handles {
        h.await.unwrap();
    }
}
```

Hint: this code may compile only on `current_thread` flavor, not multi-threaded. Why?

**Q4. Cancellation safety.**

Write an async function `transfer(from: &Account, to: &Account, amount: u64) -> Result<(), Error>` that simulates moving money between two accounts. Use a `tokio::sync::Mutex` per account. The function must:

* Lock both accounts (in a deterministic order to avoid deadlock).
* Subtract from `from`, add to `to`.
* Have at least one `.await` between the subtract and the add (simulate persistence with `sleep(...).await`).

Then, write a test that calls `transfer` inside a `tokio::time::timeout` and shows that, *if the timeout fires between the subtract and the add*, the system can be left inconsistent (money missing from `from`, never added to `to`).

Now fix it. Two acceptable approaches: (a) restructure so cancellation cannot cause inconsistency (don't hold money "in transit"), or (b) use a guard with `Drop` that rolls back on cancellation. Pick one, implement it, and explain why your approach is correct.

**Q5. CPU-bound work.**

Write an async function `compute(n: u64) -> u64` that returns the n-th Fibonacci number computed naively (recursively, not memoized — it should be slow for n > 30). Call it from a Tokio runtime in two ways:

* Version A: `compute(40).await` directly inside the async runtime.
* Version B: `tokio::task::spawn_blocking(move || compute_sync(40)).await.unwrap()`.

Spawn 10 of each version concurrently, alongside 10 instances of an unrelated task that prints "alive" every 100ms. Compare:

* How long does the whole thing take in each case?
* In Version A, do the "alive" tasks print smoothly, or do they freeze?
* Why? Explain in terms of executor threads and what `compute` is doing to them.

**Q6. Channels and select.**

Build a tiny pub/sub system. Spawn:

* A "publisher" task that emits a sequence of messages on a `tokio::sync::broadcast` channel, one every 100ms.
* Three "subscriber" tasks, each receiving from the channel.
* A "shutdown" task that, after 1 second, sends on a `tokio::sync::oneshot` channel.

Each subscriber runs a `select!` over (a) `recv()` on the broadcast channel, and (b) the shutdown signal. When shutdown fires, every subscriber prints "stopping" and exits.

Run it. Observe the output. Then deliberately introduce a slow consumer (one of the subscribers sleeps 200ms after each receive). What happens? (Hint: read the docs for `broadcast` channels and the `Lagged` error.) Adjust the design to handle this gracefully.

**Q7. Reading.**

Read the first three chapters of the official async book at https://rust-lang.github.io/async-book/. Also read the Tokio tutorial at https://tokio.rs/tokio/tutorial.

After reading, write a short paragraph each on:

* Why does Rust's async require explicit Pin and the rest of the world (Go, Python) doesn't? What design decision led to this?
* What does Tokio do that the standard library doesn't, exactly? List at least four runtime responsibilities.
* "Async Rust is a leaky abstraction." Where do the leaks show up most painfully in your experience after this drill?

---

# Lesson 6: Macros, Unsafe, and the Ecosystem

## 6.1 Why This Lesson Exists

You can write a lot of Rust without writing macros, without writing `unsafe`, and without thinking hard about which crates exist. You cannot read or use real Rust without all three.

* **Every nontrivial program uses macros.** `println!`, `vec!`, `format!`, `assert!`, `#[derive(...)]`, `#[tokio::main]` — all macros. `serde_json::json!`, `sqlx::query!`, `clap::Parser` — all macros. You won't necessarily *write* one in your first year, but you'll read code that depends on them, and when something goes wrong inside a macro expansion the error messages are bewildering until you know what's happening.
* **The standard library and many performance-critical crates use `unsafe` internally.** `Vec`, `String`, `HashMap`, every channel, every lock — implemented with internal `unsafe`. The whole point of safe Rust is that this `unsafe` is hidden behind safe APIs. Sometimes you'll need to dip in yourself, especially for FFI or for performance work where the safe abstractions impose costs. Knowing what `unsafe` *does* and *doesn't* let you do is essential.
* **The application-layer ecosystem is centred on a small set of crates.** Serde for serialization. Tokio for async. Reqwest or Hyper for HTTP. Axum or Actix-web for servers. SQLx or Diesel for SQL. Tracing for logging. Clap for CLIs. Anyhow/thiserror for errors. These crates are de facto standard. Walking into a Rust job not knowing them is like walking into a Java job not knowing Spring or Maven.

This lesson covers all three, briefly. It's not exhaustive — each topic has books written about it — but it's enough to remove the "what *is* this magic" feeling when you read real code. Treat this lesson as a tour, not a deep dive.

## 6.2 Macros: What They Actually Do

A macro is code that writes code. The Rust compiler expands every macro invocation into ordinary Rust source before doing the rest of compilation. So `println!("hello, {}", name)` doesn't exist past the early compile stage; by the time the borrow checker runs, that line has already become a much longer chunk of normal Rust that uses `std::fmt`'s machinery.

Macros are the language's escape hatch for things ordinary functions can't do:

* Variable number of arguments (`println!`, `vec!`).
* Compile-time custom syntax (`html! { <div>{content}</div> }` from the `yew` crate).
* Generating boilerplate from a small declaration (`#[derive(Serialize, Deserialize)]`).
* Compile-time string processing (`sqlx::query!` validates SQL against your schema *at compile time*).

Rust has two flavours of macros: declarative and procedural.

### Declarative macros: `macro_rules!`

The simpler kind. You define them with `macro_rules!`. They work via pattern matching on syntax — given input matching some pattern, produce output filling in a template.

A trivial example:

```rust
macro_rules! square {
    ($x:expr) => {
        $x * $x
    };
}

fn main() {
    let n = square!(5);          // expands to: 5 * 5
    let m = square!(2 + 3);      // expands to: 2 + 3 * 2 + 3, which is 11, NOT 25
}
```

The second one is a famous footgun. The macro substitutes `2 + 3` directly, so the result is `2 + 3 * 2 + 3 = 11`. The fix is parens around the expression in the template:

```rust
macro_rules! square {
    ($x:expr) => {
        ($x) * ($x)
    };
}
```

Now `square!(2 + 3)` expands to `(2 + 3) * (2 + 3)` = 25.

There's a second footgun in this version. `square!(some_expensive_call())` would expand to `(some_expensive_call()) * (some_expensive_call())`, calling the function *twice*. The hygiene-correct version captures the value in a let binding:

```rust
macro_rules! square {
    ($x:expr) => {{
        let v = $x;
        v * v
    }};
}
```

These pitfalls are why macros are a tool of last resort. Plain functions don't have evaluation-order issues. Use macros only when you can't.

A more useful example — the actual definition of `vec!` in the standard library is along these lines:

```rust
macro_rules! my_vec {
    () => { Vec::new() };
    ($($x:expr),+ $(,)?) => {{
        let mut v = Vec::new();
        $( v.push($x); )+
        v
    }};
}

fn main() {
    let a: Vec<i32> = my_vec!();
    let b = my_vec!(1, 2, 3);
    let c = my_vec!(1, 2, 3,);   // trailing comma allowed
}
```

Read this slowly:

* `()` matches no arguments. It expands to `Vec::new()`.
* `$( $x:expr ),+` matches one or more expressions separated by commas. Each match captures into the metavariable `$x`.
* `$(,)?` is "an optional trailing comma."
* In the expansion, `$( v.push($x); )+` repeats the body once per matched `$x`, with that `$x` substituted.

That's the entire trick of declarative macros: you write patterns with metavariables and repetition, and you write expansions that substitute back. It's not Turing complete — anything you can't express in patterns you can't do declaratively. For richer logic, you reach for procedural macros.

### Procedural macros (proc-macros)

Proc-macros are *Rust functions that the compiler runs at compile time*. They take in a syntax tree and produce a syntax tree. They're how `#[derive(Serialize)]` works: the proc-macro reads your struct's fields and generates an `impl Serialize` block matching them.

Three sub-kinds:

* **Custom derives**: `#[derive(MyTrait)]` runs the macro to generate a trait impl.
* **Attribute macros**: `#[my_attr]` on a function or item, used to transform it. `#[tokio::main]` is one.
* **Function-like macros**: `my_macro!(...)`, called like `macro_rules!` macros but more powerful. `sqlx::query!` is one.

You almost never write proc-macros in application code. You consume them. The crates you use most — serde, tokio, axum, sqlx, thiserror, clap — are largely proc-macro libraries.

A quick illustration of what `#[derive(Serialize)]` generates. Given:

```rust
#[derive(Serialize)]
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
}
```

The proc-macro expands to roughly (heavily simplified):

```rust
impl Serialize for Order {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        let mut state = serializer.serialize_struct("Order", 3)?;
        state.serialize_field("id", &self.id)?;
        state.serialize_field("price", &self.price)?;
        state.serialize_field("quantity", &self.quantity)?;
        state.end()
    }
}
```

You'd never want to write this by hand — it's mechanical, tedious, and error-prone. The proc-macro generates it from the struct definition. When you add a field, the impl regenerates automatically.

### Reading macro errors

Macro expansions can produce confusing errors. The compiler error points at the macro invocation site, not the underlying problem. Typical case:

```
error[E0277]: the trait bound `MyType: Serialize` is not satisfied
  --> src/main.rs:42:30
   |
42 |     let json = serde_json::to_string(&value).unwrap();
   |                ----------------------- ^^^^^ the trait `Serialize` is not implemented for `MyType`
```

The error points at your call site. The actual fix is "add `#[derive(Serialize)]` to MyType" — the trait check happened deep inside `to_string`'s generic instantiation, but the user-facing message is at your call site. Once you've seen this pattern a few times, you'll recognise "this means I forgot a derive."

Another standby: when `cargo expand` exists, use it. `cargo install cargo-expand`, then `cargo expand` shows you the source after macro expansion. Indispensable when something is going wrong inside a derive.

### When to write a macro

Almost never. Reach for them only when:

* You truly need variadic arguments that no function can express.
* You're writing a library crate where the macro saves substantial boilerplate for users (the way derive does).
* You're implementing compile-time validation (sqlx-style query checking).

For application code: write functions and structs. Use other people's macros. Don't write your own unless you have a compelling reason.

## 6.3 Unsafe Rust: What It Actually Does

A common misconception: `unsafe` "turns off the borrow checker." It does not. `unsafe` unlocks exactly five abilities:

1. Dereferencing raw pointers (`*const T`, `*mut T`).
2. Calling unsafe functions (including FFI).
3. Implementing unsafe traits (`Send`, `Sync` manually, etc.).
4. Mutating mutable static variables.
5. Accessing fields of unions.

That's the full list. Inside an `unsafe` block, the borrow checker still runs. Lifetimes still apply. Type safety still applies. The five powers above are the *only* differences.

```rust
fn main() {
    let v = vec![1, 2, 3];

    let ptr = v.as_ptr();   // *const i32 — a raw pointer

    unsafe {
        // dereferencing a raw pointer requires unsafe
        println!("{}", *ptr);             // ok: ptr is valid here
        println!("{}", *ptr.add(1));      // ok: in bounds
        // println!("{}", *ptr.add(100));    // UB: out of bounds, but compiles
    }
}
```

When you write `unsafe`, you're promising the compiler: "I have manually verified that the rules being relaxed here are still being upheld." If you're wrong, you get **undefined behaviour** — the same word C uses, with the same consequences. The program might work, might crash, might silently corrupt data, might do different things on different days. UB is a contract violation with the language itself; once it happens, all bets are off.

### The standard library's pattern

Most uses of `unsafe` in production Rust are *inside* safe abstractions. `Vec::push` calls `unsafe` code internally to write past the current end of the buffer; `Vec` ensures the capacity check happens first, so the write is safe. From the outside, `vec.push(x)` looks like normal safe Rust. Inside `Vec`, there's `unsafe` glue.

This pattern — small, audited unsafe core, large safe API around it — is how Rust scales. The standard library, the core ecosystem crates, and a handful of performance-critical bits are written this way. Application code should rarely need unsafe.

When does application code legitimately use unsafe? Three main cases:

* **FFI**: calling C libraries. By definition, the C code can't be checked by the Rust borrow checker; you mark the boundary unsafe.
* **Hardware/embedded**: poking memory-mapped registers, writing interrupt handlers. The compiler doesn't know what hardware does.
* **Last-resort optimisation**: a tight inner loop where the safe abstraction adds measurable overhead, and a small `unsafe` block buys you back the perf. Profile first; this case is rare in practice.

If you find yourself reaching for `unsafe` outside these three, look harder for a safe solution. A common beginner pattern is using `unsafe` to "get around" a borrow checker error, which is almost always wrong — the borrow checker was preventing a real bug.

### Raw pointers

`*const T` and `*mut T` are raw pointers. They're like C pointers: 8 bytes, no aliasing rules, no lifetime tracking, no automatic deref. You construct them from references, integer addresses, or other pointers, and you dereference them in `unsafe` blocks.

```rust
let x = 42i32;
let p: *const i32 = &x;          // safe: convert ref to ptr
let q: *const i32 = std::ptr::null();   // safe: null pointer

unsafe {
    println!("{}", *p);          // 42
    // println!("{}", *q);          // UB: null deref
}
```

You'll see raw pointers most often in:

* FFI signatures (`extern "C" fn` calls take/return them).
* The internals of unsafe data structures.
* Conversion to/from C strings via `CStr`/`CString`.

For application code, you'll mostly see them via FFI.

### FFI: calling C from Rust

The mechanism for using a C library:

```rust
extern "C" {
    fn strlen(s: *const u8) -> usize;
}

fn main() {
    let s = b"hello\0";   // C-style null-terminated string
    let len = unsafe { strlen(s.as_ptr()) };
    println!("{}", len);   // 5
}
```

`extern "C"` declares functions implemented in C (linked from a C library). Calling them is unsafe because the Rust compiler can't verify the C code's contract. The call site needs to ensure inputs are valid (here: that the buffer has a null terminator within reachable memory) and outputs are handled correctly (here: that the returned size makes sense).

In practice you don't write FFI bindings by hand. Tools like `bindgen` generate them from C header files. Many C libraries already have Rust bindings on crates.io — `libc`, `openssl`, `sqlite3`, `zlib`, etc. You consume the safe wrapper crates; the unsafe bridges live inside them.

### Going the other way: letting C call Rust

```rust
#[no_mangle]
pub extern "C" fn add(a: i32, b: i32) -> i32 {
    a + b
}
```

`#[no_mangle]` keeps the function's name unscrambled (Rust normally encodes type information into symbol names; FFI consumers need a stable name). `extern "C"` uses the C calling convention. With this, you can compile your Rust crate as a C-callable library (`.a` static or `.so` shared) and link it from C, Python, Go, anywhere with a C FFI.

### Miri

A tool worth knowing about: **Miri**, an interpreter that runs Rust programs and detects undefined behaviour at runtime. If you're writing unsafe code, run your tests under Miri (`cargo +nightly miri test`) to catch UB you might have missed. It's slow but precise.

## 6.4 The Application-Layer Ecosystem

The crates you'll see in 90% of Rust jobs. This is a tour, not a manual; for each, learn the basics, then consult the docs deeply when you need to use it.

### Serde: serialization for everyone

[Serde](https://serde.rs/) is the universal serialization framework. It defines two traits — `Serialize` and `Deserialize` — and a constellation of crates that target different formats (JSON, YAML, MessagePack, Bincode, CBOR, Postcard, etc.). You annotate your types with `#[derive(Serialize, Deserialize)]` once, and they work with every supported format.

```rust
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Debug)]
struct Order {
    id: u64,
    #[serde(rename = "px")]   // wire-format name differs from Rust field name
    price: i64,
    quantity: i64,
    #[serde(default)]          // missing field -> Default::default()
    note: String,
}

fn main() {
    let o = Order { id: 1, price: 100, quantity: 5, note: String::new() };

    let j = serde_json::to_string(&o).unwrap();
    println!("{}", j);
    // {"id":1,"px":100,"quantity":5,"note":""}

    let o2: Order = serde_json::from_str(&j).unwrap();
    println!("{:?}", o2);
}
```

Cargo:

```toml
[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"
```

Things to know:

* The base `serde` crate defines the traits. Format crates (`serde_json`, `serde_yaml`, `bincode`) implement the actual encoding.
* Annotations like `#[serde(rename = "...")]`, `#[serde(default)]`, `#[serde(skip)]`, `#[serde(flatten)]` cover an enormous range of customisation. The serde docs page is your friend.
* For dynamic JSON (don't know the schema), use `serde_json::Value`.

Serde is the first thing you reach for whenever data crosses a process boundary.

### Tokio + the async stack

Lesson 5 covered Tokio. The broader async stack:

* **`tokio`** — the runtime, plus async TCP, UDP, file I/O, channels, locks.
* **`hyper`** — low-level async HTTP/1 and HTTP/2.
* **`tower`** — middleware framework: services that wrap services. Used by axum/tonic.
* **`reqwest`** — high-level HTTP client. What you actually use to fetch URLs. Built on hyper.
* **`tonic`** — gRPC client and server.
* **`tokio_stream`, `futures`** — streams, async iterators, combinators.

For a typical web service: tokio + axum + reqwest + serde + tracing covers most needs.

### Axum: the standard web framework

[Axum](https://docs.rs/axum) is a web framework built on tokio + hyper + tower. It's the current default for new Rust HTTP services.

```rust
use axum::{routing::get, Router, Json};
use serde::{Deserialize, Serialize};

#[derive(Serialize)]
struct Health { status: String }

#[derive(Deserialize)]
struct Echo { message: String }

async fn health() -> Json<Health> {
    Json(Health { status: "ok".to_string() })
}

async fn echo(Json(payload): Json<Echo>) -> Json<Echo> {
    Json(payload)
}

#[tokio::main]
async fn main() {
    let app = Router::new()
        .route("/health", get(health))
        .route("/echo", axum::routing::post(echo));

    let listener = tokio::net::TcpListener::bind("0.0.0.0:3000").await.unwrap();
    axum::serve(listener, app).await.unwrap();
}
```

Notes:

* Handlers are `async fn` returning anything that implements `IntoResponse`. `Json<T>` is one such response.
* Extractors (`Json(payload): Json<Echo>`) parse incoming requests. Other extractors include `Path`, `Query`, `Form`, `State`.
* Middleware composes via tower. Logging, auth, rate limiting, compression — all available as tower layers.

Alternative: **Actix-web** is the older mainstream framework. It's still actively used, especially in performance-focused services. Axum is the more typical greenfield choice in 2026.

### Database access

Two main camps:

* **`sqlx`** — async, compile-time-checked queries. You write SQL strings, and a macro validates them against a live database connection at compile time, producing rust types from the query's columns. Catches SQL errors before deploy.
* **`diesel`** — synchronous (with async support added more recently), strongly-typed ORM. Queries are built from a Rust DSL that mirrors SQL.

Sqlx is usually preferred for new async code; Diesel for projects that want the ORM model and don't need async at the database boundary. Pick one early.

```rust
// sqlx example (PostgreSQL)
#[derive(sqlx::FromRow)]
struct Order { id: i64, price: i64, quantity: i64 }

let orders: Vec<Order> = sqlx::query_as!(
    Order,
    "SELECT id, price, quantity FROM orders WHERE price > $1",
    1000i64
)
.fetch_all(&pool)
.await?;
```

The `query_as!` macro at compile time:

1. Parses the SQL string.
2. Connects to your dev database (configured via `DATABASE_URL`).
3. Asks the database what columns the query produces and their types.
4. Generates the type-safe deserialization code.

Result: a wrong column name or type mismatch becomes a compile error, not a runtime crash.

### Tracing: structured logging

[`tracing`](https://docs.rs/tracing) is the de-facto logging/observability crate. Better than the older `log` for async code because it correctly tracks which task is logging.

```rust
use tracing::{info, warn, error, instrument};

#[instrument]
async fn process(order_id: u64) {
    info!("processing started");
    if order_id == 0 {
        warn!("invalid id");
        return;
    }
    info!(processed = true, "done");
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();
    process(42).await;
    process(0).await;
}
```

Output (default subscriber):

```
2026-04-26T... INFO process{order_id=42}: processing started
2026-04-26T... INFO process{order_id=42}: processed=true done
2026-04-26T... INFO process{order_id=0}: processing started
2026-04-26T... WARN process{order_id=0}: invalid id
```

Each log includes the active "span" (the function context), so you can trace what happened in a particular request even when many are running concurrently. Output can be JSON for ingestion by Datadog/Honeycomb/Splunk.

### Clap: command-line parsing

[`clap`](https://docs.rs/clap) is the standard CLI parser. The modern interface is derive-based:

```rust
use clap::Parser;

#[derive(Parser)]
#[command(name = "tool", version, about = "Does a thing")]
struct Cli {
    /// Path to input file
    input: String,

    /// Output file (default: stdout)
    #[arg(short, long)]
    output: Option<String>,

    /// Verbose mode
    #[arg(short, long)]
    verbose: bool,
}

fn main() {
    let cli = Cli::parse();
    if cli.verbose {
        println!("input: {}, output: {:?}", cli.input, cli.output);
    }
    // ...
}
```

You get `--help`, `--version`, automatic help text from doc comments, type validation, sub-commands, environment variable fallback, all from the derive.

### Reqwest: HTTP client

Already used in Lesson 5. The standard async HTTP client.

```rust
let client = reqwest::Client::new();
let resp = client.get("https://api.example.com/data")
    .header("Authorization", "Bearer xyz")
    .send().await?;
let body: serde_json::Value = resp.json().await?;
```

### Anyhow and thiserror: errors, again

Phase 1 introduced these. Recap:

* **`thiserror`** for libraries: define a custom error enum with annotations.
* **`anyhow`** for binaries: `Result<T, anyhow::Error>` (or the type alias `anyhow::Result<T>`) is the common return type for application functions where the error type doesn't need to be inspectable.

```rust
use anyhow::{Context, Result};

fn main() -> Result<()> {
    let config_path = std::env::var("CONFIG_PATH").context("CONFIG_PATH not set")?;
    let config = std::fs::read_to_string(&config_path)
        .with_context(|| format!("failed to read {}", config_path))?;
    // ...
    Ok(())
}
```

`.context(...)` adds an explanation that gets prepended to the error chain. When you print the error with `{:?}`, you see the whole chain: "CONFIG_PATH not set" → underlying env error. Hugely useful for debugging.

### Other names you'll see

A non-exhaustive list of crates that are common enough that you'll hit them:

* **`itertools`** — extra iterator adapters not in std (`group_by`, `chunks`, `cartesian_product`, etc.).
* **`once_cell` / `std::sync::OnceLock`** — lazy-initialised statics.
* **`uuid`** — UUID generation and parsing.
* **`chrono` / `time` / `jiff`** — date/time. The space is messy; `chrono` is most popular but has caveats; `jiff` is the modern choice.
* **`regex`** — regular expressions. Compiled at runtime; for compile-time, `lazy_regex`.
* **`rand`** — random numbers.
* **`bytes`** — efficient byte buffers, the default in async networking.
* **`parking_lot`** — replacement `Mutex` and `RwLock`, faster than std in some workloads.
* **`crossbeam`** — concurrency primitives, including the `crossbeam-channel` you may have used in Phase 1.
* **`dashmap`** — concurrent hash map.

### Cargo, workspaces, features

Quick cargo notes you'll inevitably need:

**Workspaces** group multiple related crates in one repo:

```toml
# Cargo.toml at the repo root
[workspace]
members = ["crates/server", "crates/client", "crates/shared"]
```

Each member has its own `Cargo.toml` and source tree. They can depend on each other:

```toml
# crates/server/Cargo.toml
[dependencies]
shared = { path = "../shared" }
```

Workspaces share a single target/ directory and a single Cargo.lock, dramatically speeding up multi-crate builds.

**Features** are conditional compilation flags. A crate can declare optional features and gate code behind them:

```toml
# Cargo.toml of a library
[features]
default = []
async = ["tokio"]
serde = ["dep:serde"]

[dependencies]
tokio = { version = "1", optional = true }
serde = { version = "1", optional = true }
```

```rust
// In your code
#[cfg(feature = "async")]
pub mod async_impl { /* ... */ }
```

When users add your crate, they can pick features:

```toml
mycrate = { version = "1", features = ["async", "serde"] }
```

This is how crates support optional integrations without forcing all users to pull in every dependency.

**`cargo install`** installs binary crates globally (CLIs like `cargo-expand`, `cargo-watch`, `ripgrep` if it's distributed via cargo). Distinct from `cargo add`, which adds a dependency to your project.

## 6.5 Summary: The Rules

1. **Macros are code that writes code, expanded before the rest of compilation.**
2. **`macro_rules!` is pattern-matching on syntax. Proc-macros are Rust functions that transform syntax trees.** Most application code consumes macros, doesn't define them.
3. **`#[derive(...)]` is the proc-macro pattern you'll meet most.** Serialize, Debug, Clone, etc. — boilerplate generated for you.
4. **`unsafe` unlocks five specific powers.** It does not turn off the borrow checker.
5. **Most use of `unsafe` lives inside safe abstractions.** Application code rarely needs it outside FFI.
6. **Undefined behaviour is real and unbounded.** If you write unsafe and get it wrong, the program's behaviour is no longer reasoned about.
7. **The standard ecosystem is small enough to memorise:** Serde, Tokio, Axum (or Actix), Reqwest, sqlx (or Diesel), Tracing, Clap, anyhow/thiserror.
8. **`#[derive(Serialize, Deserialize)]` from serde will solve 95% of your serialization needs.**
9. **Axum is the modern default web framework.** Tokio underneath, tower for middleware.
10. **`tracing` for logs in async code.** Don't use println in production.
11. **Workspaces for multi-crate projects. Features for optional dependencies.**

## 6.6 Drill 6

**Q1. Read a macro expansion.**

Install `cargo-expand` (`cargo install cargo-expand`). In a fresh project, write:

```rust
use serde::{Serialize, Deserialize};

#[derive(Serialize, Deserialize, Debug)]
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
}

fn main() {
    let o = Order { id: 1, price: 100, quantity: 5 };
    println!("{:?}", o);
}
```

Run `cargo expand`. Read the generated code carefully. Then answer:

* What does the `Debug` derive expand to? Trace through the generated `fmt::Debug::fmt` method.
* What does the `Serialize` derive look like? Identify where each field is serialized.
* Roughly how many lines of code did the three derives generate? How long would it have taken you to write that by hand?

**Q2. Write a tiny declarative macro.**

Implement a `hashmap!` macro that builds a `HashMap` like `vec!` builds a Vec:

```rust
let m = hashmap! {
    "one" => 1,
    "two" => 2,
    "three" => 3,
};
```

Requirements:

* Empty case: `hashmap!{}` produces an empty HashMap.
* Trailing comma allowed.
* Type inferred from contents.

Test it. Then answer: what would the equivalent function look like? What can the macro do that the function can't?

**Q3. Use `unsafe` correctly.**

Write a function `split_at_mut_two<T>(slice: &mut [T], a: usize, b: usize) -> (&mut T, &mut T)` that returns mutable references to elements `a` and `b` of a slice. The challenge: the borrow checker won't let you take two `&mut` to different elements of the same slice in safe Rust, even though it's clearly fine when `a != b`.

Implement it using `unsafe`. You'll need:

* Bounds checks (panic if `a` or `b` is out of bounds, or if they're equal).
* `slice.as_mut_ptr()` to get a `*mut T`.
* `ptr.add(i)` for the element addresses.
* `&mut *ptr` to convert pointer back to reference.

Test it works for `a < b` and `a > b`. Also test that it panics on out-of-bounds and on `a == b`.

Then answer: what *invariants* are you, the writer of the unsafe code, manually upholding? Why is the public function safe to call (i.e., why is the `unsafe` block sound)?

**Q4. Build a tiny axum service.**

Build an HTTP service with three routes:

* `GET /health` returns `{"status":"ok"}`.
* `POST /orders` accepts a JSON body `{"price": int, "quantity": int}` and returns `{"id": <generated>, "price": ..., "quantity": ...}`. Use an `Arc<Mutex<u64>>` for a global counter that generates IDs.
* `GET /orders/:id` returns a `404` with `{"error": "not found"}` for any ID. (Persistent storage is out of scope.)

Use:

* `axum` for the HTTP layer.
* `serde` for JSON.
* `tokio` for the runtime.
* `tracing` and `tracing_subscriber` for logs. Each request should log its method, path, and response time.

Run it. Curl all three endpoints. Then add an integration test (using `tokio::test` and either `reqwest` or axum's `TestClient`) that exercises the happy path of `POST /orders`.

**Q5. Read a real codebase.**

Pick any one of these crates and clone its repo:

* `tokio` (https://github.com/tokio-rs/tokio)
* `serde` (https://github.com/serde-rs/serde)
* `axum` (https://github.com/tokio-rs/axum)
* `reqwest` (https://github.com/seanmonstar/reqwest)

Spend at least one hour reading source code. You don't need to understand everything. Pick one user-facing function and trace what it does, all the way down. (Examples: `tokio::spawn`, `serde_json::to_string`, `axum::Router::route`, `reqwest::Client::get`.)

Write up your findings:

* The function's signature and what types appear.
* The first three layers of what it does (functions it calls, data structures it constructs).
* Where you stopped because it got too deep, and what you'd need to learn to continue.
* One thing you didn't know was possible in Rust before reading.

This drill matters more than the others. Reading good Rust is how you internalise idioms. Make this a habit.

**Q6. Reading.**

Read these:

* The Rust Book chapter on macros: https://doc.rust-lang.org/book/ch20-05-macros.html
* The Rustonomicon — at least the first three chapters: https://doc.rust-lang.org/nomicon/intro.html. This is *the* reference for unsafe Rust.
* The Tokio tutorial all the way through (you started it in Lesson 5): https://tokio.rs/tokio/tutorial.

Answer:

* What's the difference between "safe code with bugs" and "unsafe code with UB"? Why does the Nomicon emphasise this distinction so strongly?
* The Tokio tutorial gradually builds a Mini-Redis. After you've read it, what's one thing about real-world async design that the toy examples in this course (and Phase 1) didn't show you?
* What would you advise a new Rust hire on day 1 to ramp up fastest? Which of the things in Lesson 6 are most worth the up-front investment?

---

## Phase 2 Master Rules

A condensed reference, organised the way a senior Rust programmer would group things.

### Traits, the deep version

* Associated types are outputs of the trait; generic parameters are inputs.
* Supertraits express requirement: `trait B: A` means every `B` is also an `A`.
* The orphan rule: at least one of (trait, type) must be from your crate. Workaround: newtype.
* `impl Trait` in return position hides one concrete type; in argument position, anonymous generic.
* Most traits are object-safe (usable as `dyn Trait`); some aren't (generic methods, `Self` by value).

### Closures and iterators

* Three closure traits, in increasing flexibility: `FnOnce`, `FnMut`, `Fn`.
* The compiler auto-picks which traits a closure implements based on its captures.
* `move` changes capture mode (by value), not which traits the closure implements.
* Closures with no captures are zero-sized.
* Iterators are lazy. Adapters do nothing until a consumer drives them.
* The big four adapters: `map`, `filter`, `collect`, `fold`. Plus `take`, `skip`, `chain`, `zip`, `enumerate`, `flat_map`, `find`, `any`, `all`.
* `collect::<Result<Vec<T>, E>>()` short-circuits on the first error.
* Iterator chains compile to the same machine code as for-loops. Choose by clarity.

### Async

* A future is a state machine the runtime polls. Each `.await` is a state-machine boundary.
* Async is zero-overhead: no thread per future, often no allocation.
* The runtime is separate from the language. Use Tokio unless you have a reason not to.
* `tokio::spawn(future)` runs concurrently. `future.await` runs sequentially in the current task.
* `join!` / `join_all` for "all of these"; `select!` for "first of these"; `buffer_unordered(N)` for "fan out, capped at N."
* Don't hold non-Send across `.await` if the task might cross threads.
* Don't block the executor — use `spawn_blocking` for CPU-heavy or non-async work.
* Cancellation happens by drop. Every `.await` is a possible cancellation point.
* `tokio::sync::Mutex` if held across awaits; `std::sync::Mutex` for short critical sections.

### Macros

* Macros expand before the rest of compilation. They generate Rust source.
* Two flavours: declarative (`macro_rules!`) for pattern-based; procedural for arbitrary transformation.
* You'll consume way more macros than you write.
* `cargo expand` shows post-expansion source. Use it when macro-related errors are confusing.

### Unsafe

* `unsafe` unlocks five specific powers. It does not disable the borrow checker.
* Most production unsafe lives inside safe abstractions (Vec, HashMap, channels).
* Application code rarely needs unsafe outside FFI.
* Undefined behaviour is unbounded: when it occurs, no further reasoning about the program holds.
* Run unsafe-heavy tests under Miri.

### Ecosystem you must know

* **Serde** — serialization. `#[derive(Serialize, Deserialize)]` plus a format crate.
* **Tokio** — async runtime, plus async I/O primitives.
* **Axum** — modern web framework. Tokio + Hyper + Tower underneath.
* **Reqwest** — async HTTP client.
* **sqlx / Diesel** — database access.
* **Tracing** — structured logging in async.
* **Clap** — command-line parsing via derive.
* **anyhow / thiserror** — errors for binaries / libraries respectively.

### Tooling reflexes

* `cargo clippy` regularly. Many idiomatic improvements are caught here.
* `cargo fmt` always. Don't spend energy arguing about style.
* `cargo expand` when a macro is confusing.
* `cargo test`, `cargo bench` for tests and benchmarks.
* `cargo doc --open` for offline docs.
* Workspaces for multi-crate projects. Features for optional dependencies.

### Mindset

* When a real-world crate looks intimidating, it's almost always because it combines a few simple things. Pull on threads, look up types, read inwards.
* Most "advanced" Rust is the same Phase 1 ownership rules applied through layers. The core mental model from Phase 1 is what you keep using.
* You don't need to understand every macro and every async detail to ship code. You do need to understand them to debug code at 2 AM.
* The community emphasises documentation. Crate docs (`cargo doc`, docs.rs) are usually excellent. Read them before asking.

### Success criteria

If after Phase 2 you can:

* Read an arbitrary section of `tokio` or `serde` source and understand what it does.
* Write an HTTP service with axum, persisting to Postgres via sqlx, with structured tracing logs.
* Use iterator chains as the default and reach for for-loops only when iterators are awkward.
* Recognise the patterns in Lesson 5 well enough to debug a hung async program.
* Tell when a problem genuinely needs `unsafe` versus when it's a smell of a missing safe abstraction.
* Read crate docs and crates.io confidently when looking for a library.

Then you're a working Rust programmer. Phase 3, if there is one, is specialised by domain — embedded, kernel, WebAssembly, game engines, blockchain, scientific computing — each with its own ecosystem and tradeoffs. The Rust core stays the same.

---

*Phase 2 complete. Beyond this, the right next step depends on what you're building. For backend services: more Tokio + your database of choice + observability tooling (OpenTelemetry, Prometheus). For systems work: the Rustonomicon, embedded-rust, OS dev resources. For data: Polars, ndarray, the scientific stack. The foundations are the same; the libraries diverge.*
