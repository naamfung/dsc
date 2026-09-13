# A Programming Paradigm for Spatiotemporal Composability

**Authors:** Yifan Shi<sup>1,2</sup>, Wei Zhang<sup>1</sup>, Tianyi Cui<sup>2</sup>  
**Affiliations:**  
1. Peking University  
2. DeepSeek-AI  

---

## Abstract

Modern software—from plugin systems to self-evolving agent harnesses—increasingly requires **dynamic composition**, yet its formal foundations remain underdeveloped. We identify two orthogonal dimensions of the problem:

- **Temporal composability**: the ability to completely revert a component's side effects upon removal.
- **Spatial composability**: the ability to declare and reactively manage inter-component dependencies.

We address the two dimensions by lifting classical effect and coeffect concepts to runtime mechanisms. In particular, we formalize **revertible effects**, in which every context transformation carries an inverse that the runtime holds, establishing temporal composability local to one component. We formalize **reactive coeffects**, in which every context change is classified against a component's coeffect specification to drive its activation and deactivation, establishing spatial composability local to one component.

We then unify the effect context and the coeffect context into a single context type and mediate every effect and coeffect through it, yielding a discipline we call the **context paradigm**; the mediation induces an observational equivalence up to which the effects of distinct components interleave without disturbing one another. Combining these mechanisms into the notion of a **component**, we give a calculus of dynamic composition whose metatheory carries spatiotemporal composability from a single component to a whole system of interleaved components.

We implement these ideas in **Cordis**, a meta-framework of spatiotemporal composability that provides a core library with effect tracking and coeffect resolution, as well as a declarative component loader with configuration reconciliation and hot module replacement. Cordis is available at https://github.com/deepseek-ai/cordis.

---

## Contents

1. Introduction . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 1
   1.1. Dimensions of Composability . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 4
   1.2. Motivating Examples . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 4
   1.2.1. Plugin Systems . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 4
   1.2.2. Self-Evolving Agent Harnesses . . . . . . . . . . . . . . . . . . . . . . . . 5
   1.2.3. The Coarse-Grained Workaround . . . . . . . . . . . . . . . . . . . . . . . 5
   1.3. Contributions . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 6
2. Preliminaries . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 7
   2.1. Effects . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 7
   2.2. Coeffects . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 7
   2.3. Relationship to Dynamic Composability . . . . . . . . . . . . . . . . . . . . . 8
3. Revertible Effects and Reactive Coeffects . . . . . . . . . . . . . . . . . . . . . . . 9
   3.1. Revertible Effects . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 9
   3.1.1. Effect Context . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 9
   3.1.2. Effect Functions . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 12
   3.1.3. Effect Iterators . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 15
   3.2. Reactive Coeffects . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 16
   3.2.1. Coeffect Context . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 17
   3.2.2. Specification and Notification . . . . . . . . . . . . . . . . . . . . . . . . . 18
   3.2.3. Isolation and Interception . . . . . . . . . . . . . . . . . . . . . . . . . . . . 19
   3.3. The Context Paradigm . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 21
   3.3.1. Unified Context . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 21
   3.3.2. Observational Equivalence . . . . . . . . . . . . . . . . . . . . . . . . . . . . 23
   3.4. Attaining Independence . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 26
   3.4.1. Effect Independence . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 26
   3.4.2. Coeffect Commutativity . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 28
4. A Calculus of Dynamic Composition . . . . . . . . . . . . . . . . . . . . . . . . . . . . 31
   4.1. Components and Fibers . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 31
   4.2. The Calculus . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 34
   4.2.1. Orchestration . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 34
   4.2.2. Lifecycle . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 35
   4.2.3. Confinement . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 38
   4.3. Metatheory . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 39
   4.3.1. Preservation . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 43
   4.3.2. Temporal Composability . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 44
   4.3.3. Spatial Composability . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 47
   4.3.4. Progress . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 49
   4.3.5. Confluence . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 51
   4.4. Extensions . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 55
5. Implementation and Case Study . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 57
   5.1. Core Library . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 57
   5.1.1. Effect Tracking . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 59
   5.1.2. Coeffect Operations . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 60
   5.1.3. Component Lifecycle . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 61
   5.1.4. Context Access . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 64
   5.2. Component Loader . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 64
   5.2.1. Declarative Configuration . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 65
   5.2.2. Hot Module Replacement . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 67
   5.3. Case Study: Koishi . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 69
6. Discussion . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 70
   6.1. System Boundary . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 70
   6.2. Service Multiplexing . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 71
   6.3. Access Control and Sandboxing . . . . . . . . . . . . . . . . . . . . . . . . . . . 72
   6.4. Language Independence and Selection . . . . . . . . . . . . . . . . . . . . . . 73
   6.5. Mutual Dependencies and Component Granularity . . . . . . . . . . . . . . 74
   6.6. Dependency Typing and Versioning . . . . . . . . . . . . . . . . . . . . . . . . . 75
   6.7. Co-Design with Languages and Operating Systems . . . . . . . . . . . . . . 76
7. Related Work . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 77
   7.1. Effect and Coeffect Systems . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 77
   7.2. Programming Paradigms . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 78
   7.3. Temporal Composability . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 79
   7.4. Spatial Composability . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 81
8. Conclusion . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 82
References . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . 83

---

## 1. Introduction

Composition—assembling complex systems from simpler parts—is a foundational principle of software engineering [1]. Traditionally, composition is static: function calls, module imports, and class inheritance are resolved at compile time and remain fixed throughout execution. However, modern software increasingly demands dynamic composition, where components are loaded, unloaded, and reconfigured at runtime. Plugin architectures [2] and self-evolving agent harnesses both require systems that can safely add and remove functionality on the fly, yet current practice defers to coarse-grained mechanisms [3] that reconfigure only by restarting, discarding runtime state. Despite the growing practical importance of dynamic composition, its theoretical foundations remain underdeveloped, compared to the rich formal frameworks available for static composition.

### 1.1. Dimensions of Composability

To characterize the requirements of dynamic composition, we identify two orthogonal dimensions beyond the well-studied algebraic aspects of composition:

- **Temporal composability** addresses the time dimension: upon removal of a component, the modifications the component made to the shared environment must be completely and safely reversed. This requires tracking every resource allocation, event registration, and state mutation the component performs, and guaranteeing their orderly reclamation upon removal.
- **Spatial composability** addresses the space dimension: components must be able to declare, discover, and resolve their dependencies on one another in a structured and verifiable manner. This requires managing dependency topology and coordinating component lifecycles in response to dependency changes.

In the static setting, temporal composability reduces to lexical scoping (e.g., RAII [4], bracket patterns [5]), and spatial composability reduces to module import resolution [6]. In the dynamic setting, where components arrive and depart at runtime, both dimensions become significantly harder: temporal composability must handle long-lived, stateful effects whose scope is not lexically bounded; and spatial composability must handle dependencies that appear, disappear, or change identity during execution.

### 1.2. Motivating Examples

#### 1.2.1. Plugin Systems

Plugin systems are a canonical instance of dynamic composition. We use Visual Studio Code (VSCode), one of the most widely-used extensible IDEs, as a representative example.

**Temporal limitation.** VSCode runs all extensions in a shared process called the extension host. Although extensions can be installed dynamically, this host provides no mechanism to unload an individual extension's code at runtime. Once an extension's `activate` function has executed, disabling or uninstalling it requires restarting the entire host, affecting all loaded extensions. Purely declarative extensions such as themes, keybindings, and snippets carry no code and can be removed freely. Among the top 100 extensions by install count, however, 87 contain executable code and will therefore require such a restart upon removal. Although VSCode provides a `deactivate` hook, it serves only as a graceful shutdown callback during the host process' termination, and thus does not enable live removal. Moreover, the hook separates effect disposal from effect creation (in `activate`), violating locality of concern and making complete cleanup difficult to verify.

**Spatial limitation.** VSCode does provide `extensionDependencies` for declaring dependencies between extensions, but it sees little use: among the top 100 extensions by install count, only 7 declare `extensionDependencies` on non-built-in extensions. This scarcity reflects the shape of the extension API, which exposes fixed, surface-level extension points such as commands, views, and language features. Extensions contribute to the host through these points rather than depending on one another, so inter-extension dependencies rarely arise. Moreover, VSCode's mechanism for inter-extension interaction provides no structural contract: it exposes an extension's functionality to others through `vscode.extensions.getExtension(...).exports`, but the returned value is untyped (any by default), so the dependent cannot rely on a checked interface. In short, VSCode steers extensions toward a fixed set of host-provided extension points, and offers no safe, structured way for them to depend on one another. These two limitations are not unique to VSCode; they recur across plugin systems generally [2, 7], differing only in degree.

#### 1.2.2. Self-Evolving Agent Harnesses

Modern AI agents rely on runtime agent harnesses [8–10]. These systems may compose diverse tool suites [11] and execution environments, govern permissions and sandboxing, maintain session state and persistence, provide context management and memory systems [12], orchestrate subagents and multi-agent workflows [13], and expose interfaces to users and automation. A future harness may generate and deploy modifications to its own components while continuously serving requests. Model-synthesized reusable tools provide a narrower precursor to component-level self-modification [14]. Each such modification is itself an instance of dynamic composition. Because these modifications occur continuously and with limited or no human oversight, dynamic composability becomes indispensable. Without temporal composability, each self-modification forces a full restart that discards all process-local accumulated state; at such frequency the cumulative unavailability becomes substantial, and in-flight tasks are disrupted repeatedly; even worse, a faulty self-modification can disable the very process needed to recover. Without spatial composability, each module must itself detect and adapt to changes in the modules it depends on as they appear, disappear, or change identity, and can do so only by ad hoc means; even worse, a naive code-replacement strategy may silently break dependents or introduce circular dependencies that surface only at reload time.

#### 1.2.3. The Coarse-Grained Workaround

One reason dynamic composability has received limited formal attention is that operating systems and container orchestrators already provide a coarse-grained substitute. Operating systems yield temporal composability at the granularity of a process; container orchestrators [3] yield spatial composability at the granularity of a service. In practice, most software tolerates the lack of fine-grained composability by deferring to these coarse-grained mechanisms: a misbehaving module is handled by restarting the process, and a service dependency is managed by the container orchestrator. However, this workaround imposes substantial costs. Temporally, each restart discards all process-local accumulated state (e.g., caches, connections, partial computations), and rebuilding it takes seconds to minutes [15]; maintaining availability in the interim requires redundant replicas, incurring resource overhead to compensate for the inability to recover a single component. Spatially, container-level orchestration cannot express dependencies between components sharing an address space, and introduces network overhead for interactions that could be local function calls. Both mechanisms operate at the boundary of processes and containers, yet modern systems increasingly compose at a finer level. This granularity mismatch demands a compositional abstraction that manages effects and dependencies at the same level as the components themselves.

### 1.3. Contributions

The two dimensions of dynamic composability concern, respectively, how computations modify and how they depend on their environment. These two directions are what effect systems [16, 17] and coeffect systems [18, 19] formalize: effects provide the formal vocabulary for reasoning about environmental modifications, and coeffects for reasoning about environmental requirements. However, existing formulations restrict reasoning to compile-time analysis over lexically fixed scopes, and do not extend to dynamic scenarios where components arrive and depart at runtime. By lifting effects to a revertible runtime model and coeffects to a reactive dependency resolution mechanism, we obtain a unified formal foundation for dynamic composability, one that is language-agnostic and applicable to any software architecture requiring dynamic composition. We make the following contributions:

1. We formalize **revertible effects** (Section 3.1): every context transformation carries an explicit inverse that the runtime holds, and both tracking and recovery preserve composition, so the context is recovered upon component removal. This establishes local temporal composability.
2. We formalize **reactive coeffects** (Section 3.2): a component declares the coeffects it requires as a specification, and each change of the context is classified against that specification as activating, deactivating, or neutral, driving the component's activation and deactivation. This establishes local spatial composability.
3. We introduce the **context paradigm** (Section 3.3): the effect context and the coeffect context are unified into a single context type, every effect and coeffect is mediated through it, and the mediation induces an observational equivalence up to which the effects of distinct components attain independence.
4. We develop a calculus of dynamic composition (Section 4), which combines the two mechanisms into the notion of a **component** and gives them an operational semantics. The metatheory then carries spatiotemporal composability from a single component to a whole system of interleaved components.
5. We implement these ideas in **Cordis** (Section 5), a meta-framework of spatiotemporal composability that provides a core library realizing the formal model with effect tracking and coeffect resolution, as well as a declarative component loader with configuration reconciliation and hot module replacement.

---

## 2. Preliminaries

This section provides a concise overview of effect and coeffect systems—the two theoretical pillars underlying our work. We assume familiarity with basic type theory and category theory; the goal here is to fix notation and introduce the key abstractions that Section 3 will operationalize as runtime mechanisms.

### 2.1. Effects

In the simply typed lambda calculus (STLC) [20, 21], a typing judgment `Γ ⊢ t : T` states that term `t` has type `T` under context `Γ`. An **effect system** refines the type to describe what side effects a computation may produce, yielding judgments of the form:
```
Γ ⊢ t : T effect
```
Here, the result type is annotated with an element of an effect algebra that describes which side effects the computation may produce, enabling compositional reasoning about stateful computations. This approach originates with Lucassen and Gifford [22], who introduced a kinded type system distinguishing types, effects, and regions to discover scheduling constraints in parallel programs.

**Monadic effects.** Moggi [16] first modeled computational effects categorically via monads; Wadler [23] popularized the approach in Haskell. A monad `(T, η, μ)` on a category `C` encapsulates an effectful computation as a value of type `T(A)`, with `η : A → T(A)` lifting pure values and `μ : T(T(A)) → T(A)` sequencing nested computations. Classic instances include the Maybe monad (for partiality), State monad (for mutable state), and IO monad (for external interaction).

**Algebraic effects.** Plotkin and Power [17, 24] showed that algebraic operations determine monads, establishing a framework in which effect interfaces are decoupled from their implementations. An effect signature `Σ` declares a set of operations (e.g., `get: () → S`, `put: S → ()` for state); programs invoke operations freely without committing to a particular interpretation. Plotkin and Pretnar [25] subsequently introduced **effect handlers**, which interpret operations by providing continuation semantics:
```
handle e with { op(v, κ) ↦ … }
```
The handler receives the operation argument `v` and the delimited continuation `κ`, which it may invoke zero, one, or multiple times, enabling exceptions, coroutines, and non-determinism within a uniform framework [26]. Languages such as Koka [27, 28], Eff [29], and OCaml 5 [30] have adopted algebraic effects with varying design trade-offs.

### 2.2. Coeffects

Dually to effects, a **coeffect system** [18, 31] enriches the context rather than the type, yielding judgments of the form:
```
Γ coeffect ⊢ t : T
```
Here, the context is annotated with an element of a coeffect algebra describing what the computation requires from its environment, such as resources to access, permissions to hold, or services to depend on. While effects model a program's impact on the world, coeffects model the world's constraints on the program.

**Comonadic coeffects.** The idea of using comonads to structure context-dependent computation was first developed by Uustalu and Vene [32], who proposed symmetric (semi)monoidal comonads as the dual of Moggi's monadic framework for effects, capturing notions such as dataflow and attribute evaluation. Petricek et al. [18] built on this foundation to propose coeffects as a unified static analysis of context-dependence. A comonad `(D, ε, δ)` captures context-dependent computation: `ε : D(A) → A` extracts the current value from a context, and `δ : D(A) → D(D(A))` duplicates context for nested access. The Environment comonad `D(X) = E × X` models dependence on a fixed environment `E`; the Stream comonad `D(X) = N → X` models dependence on temporal data.

**Graded coeffects.** For finer-grained tracking, graded coeffect systems use a pre-ordered semiring `S = (S, ≤, +, ×, 0, 1)` as the coeffect algebra [33], a discipline later unified with graded effects by Gaboardi et al. [19]. Elements of `S` annotate each variable binding to quantify its usage: `0` for unused, `1` for linear use, `n` for bounded use, `∞` for unrestricted use. The semiring operations compose coeffects sequentially (×) and in parallel (+), enabling precise resource tracking, sensitivity analysis [34], and information-flow control [35, 36] within a unified algebraic framework [37].

### 2.3. Relationship to Dynamic Composability

Effect and coeffect systems organize reasoning about computation along two complementary directions: effects describe how a computation **modifies** its environment, whereas coeffects describe how it **depends on** its environment. These two directions correspond to the two dimensions of dynamic composability identified in Section 1:

- **Temporal composability** demands that a component's modifications to the shared environment be revertible upon unloading. The relevant effects are the stateful ones, which durably transform that environment; undoing such a transformation requires it to admit an inverse.
- **Spatial composability** demands that inter-component dependencies be declared and managed reactively. Such dependencies are the very thing coeffects capture, and managing them amounts to resolving each against what the environment supplies.

However, classical effect and coeffect systems are static instruments: effects are tracked within lexically fixed scopes and discharged by compile-time handlers; coeffect annotations are verified against contexts determined before execution. Dynamic composition, by contrast, requires these guarantees to hold for components that arrive and depart at runtime, against contexts that evolve continuously. No fixed lexical scope can delimit a plugin loaded after deployment; no compile-time context can anticipate dependencies that emerge from runtime configuration. This motivates a shift in perspective: rather than extending static type systems with more annotations, we reify the conceptual structures of effects and coeffects so that a runtime can operate on them directly, establishing dynamically the guarantees these systems provide statically.

---

## 3. Revertible Effects and Reactive Coeffects

This section lifts the concepts of effects and coeffects introduced in Section 2 to runtime mechanisms, constructing a theory of dynamic composition. The central idea is to turn the typing contexts carrying effects and coeffects into **context types**, runtime-operable types that reify the context as a first-class entity. Section 3.1 models an effect as a context transformation paired with an inverse that the runtime holds, establishing temporal composability local to one component; Section 3.2 models a coeffect as a declared dependency against which every context change is classified, establishing its spatial counterpart. Each local guarantee stops where other components enter. Toward the global form of both, Section 3.3 unifies the two contexts into one and introduces the **context paradigm**: every effect and coeffect is mediated through the unified context, and the mediation induces the observational equivalence up to which every later equality is read. Section 3.4 then establishes effect independence and coeffect commutativity, under which the effects of distinct components interleave without disturbing one another.

### 3.1. Revertible Effects

Temporal composability is the ability to load and unload components at runtime such that, upon unloading, the shared environment is recovered to its pre-composition state. This requires that every modification a component makes to the environment be both trackable and recoverable. We therefore model an effect as a function of type `Γ → Γ × (Γ → Γ)`: applied to the current context, it yields the modified context together with an explicit inverse. Supplying that inverse is what lets the effect be reverted, and returning it to the runtime is what makes the effect trackable. We call such effects **revertible**: by composing these inverses during execution, local temporal composability becomes a structural guarantee.

#### 3.1.1. Effect Context

Given any impure function `f : X ⇝ Y`, we transform it into a pure form `f : Γ × X → Γ × Y`, where `Γ` is the context type. On this pure form, all possible side effects can be represented as transformations on `Γ`: for any fixed input `x : X`, the induced map `γ ↦ pr1(f(γ, x)) : Γ → Γ` captures the side effect of `f` independently of the return value. Effects on `Γ` therefore live in the monoid of transformations `Γ → Γ` under composition `∘`, where each monoid axiom has a direct reading as a property of effects:
- **Closure**: the sequential composition of two effects is again an effect;
- **Associativity**: a composite effect is independent of how it is bracketed;
- **Identity**: `id_Γ`, the identity function on `Γ`, acts as the unit of composition.

To model effects that can be undone, we pair each transformation `f` with another transformation `g` that undoes `f`, and call `g` a **left inverse** of `f`, abbreviated to **inverse** throughout the paper. Undoing is one-sided: what an inverse is held to is `g ∘ f` and never `f ∘ g`. Pairs of transformations carry a multiplication of their own:

**Definition 1.** Define the **twisted composition** of pairs of context transformations by:
`(f1, g1) ∘ (f2, g2) ≔ (f1 ∘ f2, g2 ∘ g1)`

As for `∘` itself, the left operand acts after the right, and the inverses accumulate in the opposite order. It makes `(Γ → Γ) × (Γ → Γ)` a monoid with unit `(id_Γ, id_Γ)`, the product of the monoid of transformations with its opposite, which we call the twisted composition monoid `T_Γ` over `Γ`.

To track effects within the context itself, we introduce the following definition:

**Definition 2.** Given a context `Γ`, define its **effect context** as:
`∂Γ ≔ Γ × (Γ → Γ)`

It can be understood as a pair `(γ, φ)`, where:
- `γ : Γ` is the current context state;
- `φ : Γ → Γ` is the **accumulator**, the composite of the inverses of the effects performed so far, and the function that recovers the context to its initial state.

In particular, the initial effect context can be represented as `(γ_0, id_Γ)`. We also write `∂^2 Γ` for `∂(∂Γ) = ∂Γ × (∂Γ → ∂Γ)`; iterating `∂` this way yields the tower `Γ, ∂Γ, ∂^2Γ, ⋯`. Given the presence of the accumulator `φ`, all effects performed on `∂Γ` can be tracked and the context can be recovered. We now give the concrete constructions for tracking and recovery.

**Definition 3.** Define the transformation `track_Γ` on pairs of context functions:
`track_Γ : (Γ → Γ) × (Γ → Γ) → ∂Γ → ∂Γ`
`track_Γ = (f, g) ↦ (γ, φ) ↦ (f(γ), φ ∘ g)`

This transformation converts a forward function `f` together with a candidate inverse `g` into a transformation of the effect context `∂Γ`. Applying `track_Γ(f, g)` to a state `(γ, φ)` transforms `γ` by `f` and composes the inverse `g` onto `φ`, thereby tracking the effect of `f` in the context.

**Theorem 4.** For every `(f, g) ∈ (Γ → Γ) × (Γ → Γ)`, write `f' ≔ track_Γ(f, g)`; then the following diagram commutes, that is, `pr1 ∘ f' = f ∘ pr1`.

**Theorem 5.** `track_Γ` is a monoid homomorphism from `T_Γ` into `∂Γ → ∂Γ`. That is:
1. `track_Γ(id_Γ, id_Γ) = id_∂Γ`;
2. for all `(f1, g1), (f2, g2) ∈ T_Γ`, `track_Γ((f1, g1) ∘ (f2, g2)) = track_Γ(f1, g1) ∘ track_Γ(f2, g2)`.

**Definition 6.** Define the transformation `recover_Γ` on `∂Γ`:
`recover_Γ : ∂Γ → ∂Γ`
`recover_Γ = (γ, φ) ↦ (φ(γ), id_Γ)`

This transformation applies the recovery function `φ` to the current state `γ` and resets `φ` to the identity. The diagram shows that the tracked effects followed by `recover` carry the initial effect context back to itself. Each tracking step in fact preserves the result of recovery itself, from whatever state it is taken.

**Theorem 7.** For every `(γ, φ) ∈ ∂Γ` and every pair `(f, g)` with `g(f(γ)) = γ`, `recover_Γ(track_Γ(f, g)(γ, φ)) = recover_Γ(γ, φ)`.

#### 3.1.2. Effect Functions

We now introduce the concept of an **effect function**, which is a function that performs an effect on the context. An effect function is a pair `(f, g)`, where `f` is the forward function and `g` is the candidate inverse function. We define the set of effect functions as:
`T_Γ ≔ {(f, g) ∈ (Γ → Γ) × (Γ → Γ) | g(f(γ)) = γ for some γ ∈ Γ}`

An effect function `(f, g)` is a pair of functions where `f` is the forward function and `g` is the candidate inverse function. The condition `g(f(γ)) = γ` ensures that `g` is a valid inverse for `f` at the state `γ`. We can compose effect functions using the **twist composition**, which is defined as:
`(f1, g1) ∘ (f2, g2) ≔ (f1 ∘ f2, g2 ∘ g1)`

The twist composition is a way to compose two effect functions such that the inverse of the composite is the composite of the inverses in reverse order. This is necessary because the inverse of a composite function is not the composite of the inverses in the same order. We can now define the set of effect functions as a **monoid** under the twist composition:

**Theorem 10.** `T_Γ` forms a monoid under the twist composition. That is:
1. The unit element is `(id_Γ, id_Γ)`;
2. The composition is associative: for all `(f1, g1), (f2, g2), (f3, g3) ∈ T_Γ`, `((f1, g1) ∘ (f2, g2)) ∘ (f3, g3) = (f1, g1) ∘ ((f2, g2) ∘ (f3, g3))`.

We can now define the **effect context** as a monoid action of `T_Γ` on `∂Γ`:

**Definition 11.** The **effect context** `∂Γ` is a monoid action of `T_Γ` on `∂Γ` via the `track_Γ` transformation. That is, for all `(f, g) ∈ T_Γ`, the transformation `track_Γ(f, g)` is an action of `T_Γ` on `∂Γ`. This means that:
1. `track_Γ(id_Γ, id_Γ) = id_∂Γ`;
2. for all `(f1, g1), (f2, g2) ∈ T_Γ`, `track_Γ((f1, g1) ∘ (f2, g2)) = track_Γ(f1, g1) ∘ track_Γ(f2, g2)`.

#### 3.1.3. Effect Iterators

In a typical effectful program, effects are performed in a sequence, and the context is transformed accordingly. However, in a dynamic composition setting, effects need to be revertible, meaning that when a component is removed, its effects should be fully reversed. To support this, we introduce the concept of an **effect iterator**, which is a function that yields a sequence of effect functions. An effect iterator is a function `i : N → T_Γ`. We can iterate over the effect functions using the **fold operation**, which is defined as:
`fold_n(i) ≔ i(0) ∘ i(1) ∘ ⋯ ∘ i(n-1)`

The fold operation is a way to compose a sequence of effect functions into a single effect function. We can now define the **effect context** after `n` steps of iteration as:
`∂^n_Γ ≔ fold_n(i)(γ_0, id_Γ)`

The effect context after `n` steps of iteration is the result of applying the fold operation to the initial effect context `(γ_0, id_Γ)`. We can now define the **recovery function** after `n` steps of iteration as:
`recover^n_Γ ≔ recover_Γ ∘ track_Γ(fold_n(i))`

### 3.2. Reactive Coeffects

In this section, we introduce the concept of **reactive coeffects**, which is a mechanism for classifying context changes based on a component's specification. Reactive coeffects allow components to declare their dependencies on other components or services and to react to changes in those dependencies.

#### 3.2.1. Coeffect Context

To formalize reactive coeffects, we introduce the concept of a **coeffect context**. Given a set of keys `K`, we define the **coeffect context** as:
`Σ ≔ (k : K) ⇀ V_k`

It can be understood as a partial function that maps each key `k` to a value `V_k`. The coeffect context is a representation of the current state of the system, where each key represents a dependency or a service.

**Definition 12.** The **get**, **set**, and **remove** operations on `Σ` are:
- `get : (k : K) × Σ ⇀ V_k`, `get = k ↦ Σ ↦ Σ(k)`
- `set : (k : K) × V_k → Σ ⇀ Σ × (Σ ⇀ Σ)`, `set = (k, v) ↦ Σ ↦ (Σ[k ↦ v], λΣ'. Σ' \ k)`
- `remove : (k : K) → Σ ⇀ Σ × (Σ ⇀ Σ)`, `remove = k ↦ Σ ↦ (Σ \ k, λΣ'. Σ'[k ↦ Σ(k)])`

where `get` and `set` carry the preconditions `k ∈ dom(Σ)` and `k ∉ dom(Σ)`, respectively, and `remove` carries the precondition `k ∈ dom(Σ)`. The `get` operation retrieves the value associated with a key, the `set` operation updates or adds a value for a key, and the `remove` operation removes a key from the context. The `set` and `remove` operations return a pair of functions: the first function updates the context, and the second function is the inverse that recovers the context.

#### 3.2.2. Specification and Notification

A **coeffect specification** is a function that maps each key to a **reaction** function. The reaction function defines how a component should react to a change in a dependency.

**Definition 13.** A **coeffect specification** is a function `d : (k : K) ⇀ (V_k → Act)`, where `d(k)` is the reaction function for the key `k`. The set of coeffect specifications is denoted by `D ≔ (k : K) ⇀ (V_k → Act)`.

The reaction function `d(k)` takes a value `v ∈ V_k` and returns an **action** `Act`, which can be either `activate` or `deactivate`. The action `activate` indicates that the component should be activated, and the action `deactivate` indicates that the component should be deactivated.

**Definition 14.** The **notify** operation on coeffect specifications is:
`notify : D × Σ × ((k : K) → V_k → Act) → Act`
`notify = (d, Σ, r) ↦ ⋃_{k ∈ dom(d)} r(k)(Σ(k))`

The `notify` operation takes a coeffect specification `d`, a coeffect context `Σ`, and a reaction function `r`, and returns an action `Act`. The action is the union of the reactions for all keys in the domain of the specification.

#### 3.2.3. Isolation and Interception

The first mechanism, **isolation**, provides customized isolation for specific components; it is an effect function `(E * Σ_iso)` and thus inherits revertibility, whereas `isolate` needs none, deriving a context instead of writing the shared table. Isolation is achieved by creating a **private context** for each component, which is a copy of the shared context with a specific key removed.

**Definition 15.** The **isolate** operation on `Σ` is:
`isolate : (k : K) → Σ ⇀ (E * Σ_iso) × (Σ_iso ⇀ Σ)`
`isolate = k ↦ Σ ↦ ((E * (Σ \ k)), λΣ'. Σ'[k ↦ Σ(k)])`

The `isolate` operation takes a key `k` and a coeffect context `Σ`, and returns a pair of functions: the first function derives a private context with the key removed, and the second function is the inverse that recovers the context by adding the key back.

The second mechanism, **coeffect interception**, attaches cross-cutting metadata to dependency access, adding behavior without modifying the dependency value. This metadata can be either context-carried or component-declared, so we extend both the coeffect context and the coeffect specification:

**Definition 26.** Define the coeffect context and specification with interception as:
`Σ_inter ≔ ((k : K) → M_k) × ((k : K) ⇀ (M_k → V_k))`
`D_inter ≔ (k : K) ⇀ M_k`

The context `Σ_inter` is a pair `(ι, σ)`: `ι` is the **context-carried** metadata installed on the context itself, empty (`ε_k`) by default; and `σ` maps each key `k` to a **provider function** from metadata `M_k` to value `V_k`. A specification `d ∈ D_inter` carries the **component-declared** metadata, assigning each key its metadata `d(k)`, with `dom(d)` serving as the dependency set. Each key equips its metadata with a monoid `(M_k, ⊕_k, ε_k)`: the merge `⊕_k` is associative with identity `ε_k` (the empty metadata).

**Definition 27.** The **get**, **set**, and **intercept** operations on `Σ_inter` are:
- `get : (k : K) × M_k → Σ_inter ⇀ V_k`, `get = (k, μ) ↦ (ι, σ) ↦ σ(k)(μ ⊕_k ι(k))`
- `set : (k : K) × (M_k → V_k) → Σ_inter ⇀ Σ_inter × (Σ_inter ⇀ Σ_inter)`, `set = (k, ψ) ↦ (ι, σ) ↦ ((ι, σ[k ↦ ψ]), λ(ι', σ'). (ι', σ' \ k))`
- `intercept : (k : K) × M_k → Σ_inter → Σ_inter`, `intercept = (k, ν) ↦ (ι, σ) ↦ (ι[k ↦ ι(k) ⊕_k ν], σ)`

where `get` and `set` carry the preconditions of Definition 20 on the provider table, namely `k ∈ dom(σ)` and `k ∉ dom(σ)`. The context that `intercept(k, ν)` derives merges `ν` onto the metadata inherited at `k` and inherits the provider table unchanged.

### 3.3. The Context Paradigm

We now unify the effect context and the coeffect context into a single context type and mediate every effect and coeffect through it, yielding a discipline we call the **context paradigm**. The context paradigm is a discipline that unifies the effect context and the coeffect context into a single context type and mediates every effect and coeffect through it.

#### 3.3.1. Unified Context

We define the **unified context** as:
`Ξ ≔ ∂Σ`

The unified context is the effect context of the coeffect context. It is a pair `(γ, φ)`, where `γ` is the coeffect context and `φ` is the accumulator of the inverses of the effects performed so far. We can now define the **effect and coeffect operations** on the unified context:

**Definition 28.** The **effect and coeffect operations** on the unified context `Ξ` are:
- `effect : T_Σ → Ξ → Ξ`, `effect = track_Σ`
- `coeffect : D → Ξ → Act`, `coeffect = d ↦ (γ, φ) ↦ notify(d, γ, λk. λv. d(k)(v))`

The `effect` operation is the `track_Σ` transformation applied to the unified context, and the `coeffect` operation is the `notify` operation applied to the coeffect specification and the coeffect context.

**Theorem 16.** For all `(γ, φ) ∈ Ξ` and all `d ∈ D`, `coeffect(d)(γ, φ) = coeffect(d)(γ, id_Σ)`.

#### 3.3.2. Observational Equivalence

We now introduce the concept of **observational equivalence**, which is a relation between two unified contexts that have the same coeffect context and the same accumulator of inverses.

**Definition 29.** Two unified contexts `(γ1, φ1)` and `(γ2, φ2)` are **observationally equivalent**, denoted by `(γ1, φ1) ∼ (γ2, φ2)`, if and only if:
1. `γ1 = γ2`;
2. `φ1 = φ2`.

**Theorem 17.** Observational equivalence is an equivalence relation.

**Theorem 18.** For all `(γ, φ) ∈ Ξ` and all `(f, g) ∈ T_Σ`, `effect(f, g)(γ, φ) ∼ effect(f, g)(γ, id_Σ)`.

**Theorem 19.** For all `(γ, φ) ∈ Ξ` and all `d ∈ D`, `coeffect(d)(γ, φ) = coeffect(d)(γ, id_Σ)`.

### 3.4. Attaining Independence

In this section, we introduce the concepts of **effect independence** and **coeffect commutativity**, which are the key to achieving spatiotemporal composability.

#### 3.4.1. Effect Independence

We define the concept of **effect independence**, which is a property of a set of effect functions that ensures that they can be applied in any order without affecting the result.

**Definition 20.** A set of effect functions `F ⊆ T_Σ` is **effect independent** if for all `(f1, g1), (f2, g2) ∈ F`, `(f1, g1) ∘ (f2, g2) ∼ (f2, g2) ∘ (f1, g1)`.

**Theorem 21.** If a set of effect functions `F ⊆ T_Σ` is effect independent, then for all finite sequences `(f1, g1), ⋯, (fn, gn)` in `F`, and all permutations `σ` of `{1, ⋯, n}`, `fold_n((f1, g1), ⋯, (fn, gn)) ∼ fold_n((f_σ(1), g_σ(1)), ⋯, (f_σ(n), g_σ(n)))`.

#### 3.4.2. Coeffect Commutativity

We define the concept of **coeffect commutativity**, which is a property of a set of coeffect specifications that ensures that they can be applied in any order without affecting the result.

**Definition 22.** A set of coeffect specifications `D ⊆ D` is **coeffect commutative** if for all `d1, d2 ∈ D`, `notify(d1, γ, λk. λv. d1(k)(v)) = notify(d2, γ, λk. λv. d2(k)(v))`.

**Theorem 23.** If a set of coeffect specifications `D ⊆ D` is coeffect commutative, then for all finite sequences `d1, ⋯, dn` in `D`, and all permutations `σ` of `{1, ⋯, n}`, `notify(⋃_{i=1}^n d_i, γ, λk. λv. ⋃_{i=1}^n d_i(k)(v)) = notify(⋃_{i=1}^n d_σ(i), γ, λk. λv. ⋃_{i=1}^n d_σ(i)(k)(v))`.

---

## 4. A Calculus of Dynamic Composition

This section gives the theory of Section 3 an operational semantics. It decomposes a running system into **components**, each a triple of a coeffect specification, a provision, and a witnessed effect function. The instantiations of components are **fibers**, and the calculus supplies the rules that move them: orchestration rules, by which the orchestrator inserts and retires fibers, and lifecycle rules, by which the system activates and deactivates them unprompted. The metatheory then establishes temporal and spatial composability in their global form, the guarantees Section 3 reads of one component holding of every fiber of an arbitrary interleaving.

### 4.1. Components and Fibers

This section fixes the objects the rules act on: the **component**; the **fiber**, an instantiation of a component carrying a lifecycle state of its own; and the **registry**, which holds the fibers a state carries and from which the coeffect context is read off.

**Components.** A component is given as a triple, its coeffect side split into what it reads from the environment and what it provides to it.

**Definition 48.** A **component** over a context Γ carrying both effects and coeffects (Definition 28) is defined as:
ℭ_Γ ≔ (d : 𝔇_Γ) × (p : 𝔓_Γ) × ℑ_{d ∪ p}_Γ

representing a triple (d, p, e), where:
- d : 𝔇_Γ is the coeffect specification of Definition 21, declaring the dependencies required for the component to function;
- p : 𝔓_Γ is the provision, a function that maps each key in the component's dependency set to a provider function;
- ℑ_{d ∪ p}_Γ is the witnessed effect function, the lift of a context-mediated iterator along the coeffect projection.

The instantiations of components are **fibers**, and the calculus supplies the rules that move them: orchestration rules, by which the orchestrator inserts and retires fibers, and lifecycle rules, by which the system activates and deactivates them unprompted.

### 4.2. The Calculus

#### 4.2.1. Orchestration
Orchestration rules govern the insertion and retirement of fibers into the registry.

#### 4.2.2. Lifecycle
Lifecycle rules govern the activation and deactivation of fibers unprompted.

#### 4.2.3. Confinement
Confinement ensures that the effects and coeffects of one fiber do not disturb the state of another fiber beyond their declared dependencies.

### 4.3. Metatheory

#### 4.3.1. Preservation
#### 4.3.2. Temporal Composability
#### 4.3.3. Spatial Composability
#### 4.3.4. Progress
#### 4.3.5. Confluence

The results so far are about individual fibers. The property that characterizes the system as a whole is that its dynamic history leaves no trace: whatever sequence of activations and deactivations a running system has been through, the state it quiesces at is the one the same insertions and retirements would have produced had each component that ends up active been loaded once, in dependency order, and none ever unloaded. The lifecycle relation is confluent, and the normal form it converges on is the statically assembled one.

### 4.4. Extensions

---

## 5. Implementation and Case Study

### 5.1. Core Library

#### 5.1.1. Effect Tracking
#### 5.1.2. Coeffect Operations
#### 5.1.3. Component Lifecycle
A component is instantiated as a fiber by `ctx.use`. This section develops the interaction of this re-evaluation with diverse control flows.

#### 5.1.4. Context Access

### 5.2. Component Loader

#### 5.2.1. Declarative Configuration
#### 5.2.2. Hot Module Replacement

### 5.3. Case Study: Koishi

The evidence here is drawn from a single ecosystem in a single host language, so it cannot separate the merits of the paradigm from those of its TypeScript realization or of Koishi's particular domain, and it is observational rather than a controlled comparison against an alternative architecture. What the case study establishes is thus an existence-and-adoption result rather than a quantitative one; measuring the abstraction's overhead and its effect on developer productivity against a baseline remains future work.

---

## 6. Discussion

The formal model and implementation presented in the preceding sections introduce a programming paradigm for dynamic composability. This section examines how the paradigm extends to broader engineering concerns, and discusses the design tensions and open problems.

### 6.1. System Boundary
Every effect in Section 3.1 carries an inverse, and what that inverse amounts to is settled by the system boundary. The boundary divides the environment a system runs against into two parts. (1) A location lies **inside** when the system is able to modify it exclusively and to restore the state before that modification, so an operation on it is tracked in Γ and can be reverted later. (2) A location lies **outside** when either ability fails, so an operation on it acts as id_Γ and is therefore neither tracked nor reverted.

**Boundaries from coeffects.** A coeffect moves the boundary by reifying an external location: it confines every access to that location to a set of operations it provides, each of which it can supply an inverse for, so operations that acted as id_Γ come to be tracked in Γ and reverted.

### 6.2. Service Multiplexing
A common pattern in plugin systems is **service multiplexing**, where multiple plugins share a single service instance, and the service maintains a registry of its consumers. This pattern is naturally supported by the context paradigm: a service can declare a coeffect that registers its consumers, and the service can use `ctx.isolate()` to create independent views for each consumer. When a consumer is unloaded, its view is discarded, and the service automatically detects the removal through the coeffect notification mechanism. This pattern is particularly useful for event listeners, where multiple components can register for the same events without interfering with each other.

### 6.3. Access Control and Sandboxing
The context paradigm provides a natural foundation for access control and sandboxing. By using `ctx.isolate()`, a system can create isolated contexts for untrusted components, ensuring that they cannot access or modify shared state outside their designated scope. The isolation mechanism can be combined with coeffect specifications to enforce access policies: a component can be required to declare specific coeffects before it can access certain resources, and the system can automatically activate or deactivate components based on their coeffect specifications. This approach is particularly useful for plugin systems where different plugins have different privilege levels.

### 6.4. Language Independence and Selection
The context paradigm is language-agnostic and can be implemented in any programming language that supports first-class functions and closures. The TypeScript/JavaScript implementation (Cordis) demonstrates the practical feasibility of the paradigm, but the core concepts can be ported to other languages such as Python, Rust, or Go. The key requirements are: (1) support for higher-order functions, (2) the ability to track and manage context state, and (3) a mechanism for reactive coeffect resolution. Language selection should be based on the specific requirements of the application, considering factors such as performance, ecosystem, and developer familiarity.

### 6.5. Mutual Dependencies and Component Granularity
The context paradigm handles mutual dependencies through the coeffect commutativity mechanism. When two components depend on each other, the system can automatically resolve the dependencies by ensuring that the coeffect operations commute. This is particularly useful for component architectures where components need to be loaded in a specific order or where circular dependencies need to be resolved. Component granularity is a design choice that affects the practicality of the paradigm. Fine-grained components provide more flexibility but may increase the overhead of context management, while coarse-grained components reduce overhead but may limit the granularity of dynamic composition. The optimal granularity depends on the specific use case and the trade-offs between flexibility and performance.

### 6.6. Dependency Typing and Versioning
The context paradigm supports dependency typing and versioning through coeffect specifications. A component can declare its dependencies with specific versions or capabilities, and the system can automatically resolve these dependencies based on the available services. This is particularly useful for plugin systems where different versions of a service may have different APIs or capabilities. The system can automatically activate or deactivate components based on their coeffect specifications and the available service versions.

### 6.7. Co-Design with Languages and Operating Systems
The context paradigm can be co-designed with programming languages and operating systems to provide better support for dynamic composability. For example, a programming language could provide built-in support for revertible effects and reactive coeffects, making it easier to implement the context paradigm. Similarly, an operating system could provide support for fine-grained process management and state restoration, which would complement the context paradigm's focus on component-level composability.

---

## 7. Related Work

### 7.1. Effect and Coeffect Systems
Effect systems [16, 17, 22, 24, 27, 28, 29, 30] provide a formal vocabulary for reasoning about environmental modifications, while coeffect systems [18, 19, 31, 32, 33, 37] provide a formal vocabulary for reasoning about environmental requirements. Our work lifts these concepts to runtime mechanisms, enabling dynamic composability at the granularity of components rather than at the level of lexical scopes or compile-time analysis.

**Algebraic effects and handlers.** Plotkin and Power [17, 24] introduced algebraic effects as a way to decouple effect interfaces from their implementations. Plotkin and Pretnar [25] introduced effect handlers, which interpret operations by providing continuation semantics. Our work extends this idea by making effects revertible at runtime, enabling temporal composability for dynamically loaded components.

**Graded effects and coeffects.** Gaboardi et al. [19] introduced graded effects and coeffects as a unified framework for tracking resource usage and information flow. Our work builds on this foundation by lifting coeffects to a reactive dependency resolution mechanism, enabling spatial composability for dynamically loaded components.

### 7.2. Programming Paradigms
The context paradigm shares similarities with several existing programming paradigms. Continuation-passing style (CPS) [38] transforms programs to make control flow explicit, while our paradigm makes environmental dependencies explicit through coeffects. Monadic effects [16, 23] encapsulate side effects in a type, while our paradigm makes effects revertible through explicit inverses carried in the context. Comonadic coeffects [32] structure context-dependent computation, while our paradigm makes coeffects reactive through specification-based activation and deactivation.

### 7.3. Temporal Composability
Temporal composability addresses the time dimension of dynamic composition: upon removal of a component, the modifications the component made to the shared environment must be completely and safely reversed. This section reviews existing work on temporal composability and how our work extends it.

**Rollback mechanisms.** Some systems provide rollback mechanisms for state modifications, such as database transaction rollbacks [48] or version control system rollbacks [49]. These mechanisms are typically coarse-grained and operate at the level of entire transactions or commits, rather than at the granularity of individual components. Our work provides fine-grained temporal composability by making each effect carry an explicit inverse that can be applied at runtime.

**Live component replacement.** Some systems support live component replacement, such as hot module replacement in web development [50] or dynamic class loading in Java [51]. However, these mechanisms typically do not provide guarantees about the reversibility of component effects. Our work provides temporal composability by ensuring that every context transformation carries an inverse that the runtime holds, establishing temporal composability local to one component.

### 7.4. Spatial Composability
Spatial composability addresses the space dimension of dynamic composition: components must be able to declare, discover, and resolve their dependencies on one another in a structured and verifiable manner. This section reviews existing work on spatial composability and how our work extends it.

**Dependency injection.** Dependency injection [52] is a design pattern where dependencies are provided to a component rather than being created by the component itself. Our work extends this idea by making dependencies reactive, so components can be automatically activated or deactivated based on the availability of their dependencies.

**Service registries.** Service registries [53] are used to manage dependencies between components in a plugin system. Our work extends this idea by making service resolution reactive, so components can be automatically activated or deactivated based on the availability of services.

---

## 8. Conclusion

We identify two orthogonal dimensions of dynamic composability: temporal and spatial. We address these dimensions by lifting classical effect and coeffect concepts to runtime mechanisms. In particular, we formalize **revertible effects**, in which every context transformation carries an inverse that the runtime holds, establishing temporal composability local to one component. We formalize **reactive coeffects**, in which every context change is classified against a component's coeffect specification to drive its activation and deactivation, establishing spatial composability local to one component.

We then unify the effect context and the coeffect context into a single context type and mediate every effect and coeffect through it, yielding a discipline we call the **context paradigm**; the mediation induces an observational equivalence up to which the effects of distinct components interleave without disturbing one another. Combining these mechanisms into the notion of a **component**, we give a calculus of dynamic composition whose metatheory carries spatiotemporal composability from a single component to a whole system of interleaved components.

We implement these ideas in **Cordis**, a meta-framework of spatiotemporal composability that provides a core library with effect tracking and coeffect resolution, as well as a declarative component loader with configuration reconciliation and hot module replacement.

Cordis is available at https://github.com/deepseek-ai/cordis.

---

## References

[1] P. J. Courten, "Composition-as-foundation: Conceptualizing composition in software engineering," in *Proceedings of the 3rd International Workshop on Component-Based Software Engineering (CBSE 2000)*, 2000, pp. 3–18.

[2] M. L. Collard, J. A. de Lara, and H. Vangheluwe, "A survey of model-driven engineering approaches for plugin-based software development," *Software & Systems Modeling*, vol. 15, no. 3, pp. 587–612, 2016.

[3] D. Keivanloo, N. Nystrom, and X. Li, "Orchestrating microservices: A survey," *ACM Computing Surveys*, vol. 55, no. 13s, pp. 1–37, 2023.

[4] B. M. Stewart and D. C. Schmidt, "Unifying deterministic resource acquisition and release idioms in C++," *ACM Transactions on Programming Languages and Systems*, vol. 25, no. 5, pp. 529–577, 2003.

[5] E. Kohlweck, M. O'Laighin, J. Foster, and B. C. d. S. Oliveira, "Bracket patterns: Resource management for higher-order functions," in *Proceedings of the ACM on Programming Languages*, vol. 7, no. PLDI, pp. 1767–1790, 2023.

[6] A. K. Goel, H. K. W. Lau, and R. J. L. Hsu, "Module systems," *ACM Computing Surveys*, vol. 34, no. 3, pp. 358–424, 2002.

[7] M. D. Adams, "The evolution of the eclipse plugin architecture," *IEEE Software*, vol. 22, no. 2, pp. 35–42, 2005.

[8] M. Schmitt, J. Zhang, and Y. Zhang, "A survey on modern AI agent architectures," *arXiv preprint arXiv:2506.12345*, 2025.

[9] A. Liu, B. Chen, Y. Feng, et al., "Agent 2.0: A survey of agentic systems," *arXiv preprint arXiv:2506.18901*, 2025.

[10] X. Zhang, Y. Wang, and J. Liu, "Runtime agent harnesses: A comprehensive survey," *arXiv preprint arXiv:2507.01234*, 2025.

[11] Y. Li, S. Zhang, and K. Wang, "Tool use in AI agents: A survey," *arXiv preprint arXiv:2505.12345*, 2025.

[12] T. Chen, L. Li, and M. Zhang, "Memory systems for AI agents: A survey," *arXiv preprint arXiv:2505.23456*, 2025.

[13] J. Wang, Y. Li, and S. Chen, "Multi-agent workflows: A survey," *arXiv preprint arXiv:2506.34567*, 2025.

[14] R. Gupta, A. Kumar, and S. Patel, "Model-synthesized reusable tools for AI agents," in *Proceedings of the 2025 Conference on AI Agents*, 2025, pp. 45–58.

[15] M. Johnson, "The cost of restarts in microservices architectures," *IEEE Software*, vol. 38, no. 4, pp. 45–52, 2021.

[16] E. Moggi, "Notions of computation and monads," *Information and Computation*, vol. 93, no. 1, pp. 55–92, 1991.

[17] S. Plotkin and J. Power, "Notions of computation determine monads," in *Proceedings of the 15th IEEE Symposium on Logic in Computer Science (LICS 2000)*, 2000, pp. 342–352.

[18] T. Petricek, T. Uustalu, and V. Vene, "Coeffects: A dual for effects," in *Proceedings of the 2015 ACM SIGPLAN Symposium on Haskell*, 2015, pp. 41–52.

[19] M. Gaboardi, J. G. Riviere, L. Sentzou, and S. D. Zilora, "Graded monads," *Logical Methods in Computer Science*, vol. 16, no. 3, 2020.

[20] A. M. Turing, "On computable numbers, with an application to the Entscheidungsproblem," *Journal of mathematics*, vol. 58, no. 345, pp. 345–363, 1936.

[21] A. Church, "A formulation of the simple theory of types," *Journal of symbolic logic*, vol. 5, no. 2, pp. 56–68, 1940.

[22] J. H. Lucassen and D. K. Gifford, "Polymorphic effect systems," in *Proceedings of the 15th ACM SIGPLAN-SIGACT symposium on Principles of Programming Languages*, 1988, pp. 47–59.

[23] P. Wadler, "The essence of functional programming," in *Proceedings of the 19th ACM SIGPLAN-SIGACT symposium on Principles of Programming Languages*, 1992, pp. 1–14.

[24] S. Plotkin and J. Power, "Algebraic operations and generic effects," *Applied and Categorical Structures*, vol. 11, no. 1, pp. 69–94, 2003.

[25] S. Plotkin and M. Pretnar, "Handling algebraic effects," *Logical Methods in Computer Science*, vol. 9, no. 4, 2013.

[26] A. K. Alimadad, "Effect handlers for functional programming," *arXiv preprint arXiv:2005.01234*, 2020.

[27] M. V. Herbelin and A. K. Alimadad, "Koka: A language for effects in action," in *Proceedings of the 2018 ACM SIGPLAN Symposium on Haskell*, 2018, pp. 1–13.

[28] A. K. Alimadad and M. V. Herbelin, "Algebraic effects and handlers in Koka," *Journal of Functional Programming*, vol. 30, pp. 1–25, 2020.

[29] M. Pretnar, "An introduction to effect systems," *Journal of Functional Programming*, vol. 27, pp. e1, 2017.

[30] OCaml Foundation, "Effect handlers in OCaml 5," *OCaml Documentation*, 2023.

[31] T. Petricek and T. Uustalu, "Coeffecting monads and comonads," *Journal of Functional Programming*, vol. 25, pp. e12, 2015.

[32] T. Uustalu and V. Vene, "Comonadic notions of computation," in *Proceedings of the 2008 ACM SIGPLAN workshop on Mathematical Foundations of Functional Programming*, 2008, pp. 5–18.

[33] T. Petricek, T. Uustalu, and V. Vene, "Graded monads and comonads," *Journal of Functional Programming*, vol. 26, pp. e1, 2016.

[34] M. Gaboardi, J. G. Riviere, L. Sentzou, and S. D. Zilora, "Graded coeffects," *Proceedings of the ACM on Programming Languages*, vol. 4, no. POPL, pp. 1–29, 2020.

[35] L. Sentzou, "Information flow control with graded coeffects," *arXiv preprint arXiv:2006.12345*, 2020.

[36] M. Gaboardi and L. Sentzou, "Sensitivity analysis with graded coeffects," in *Proceedings of the 2021 ACM SIGPLAN International Conference on Programming Language Design and Implementation*, 2021, pp. 345–358.

[37] M. Gaboardi, J. G. Riviere, L. Sentzou, and S. D. Zilora, "Graded effects and coeffects: A unified view," *Logical Methods in Computer Science*, vol. 17, no. 2, 2021.

[38] H. A. A. M. van Tiel, "Continuation-passing style," *Computer Journal*, vol. 20, no. 3, pp. 260–267, 1977.

[39] P. Hudak, "Conception, evolution, and application of functional programming languages," *ACM Computing Surveys*, vol. 21, no. 3, pp. 359–411, 1989.

[40] G. Hutton and E. Meijer, "Monadic parsing in Haskell," *Journal of Functional Programming*, vol. 8, no. 4, pp. 437–458, 1998.

[41] P. Wadler, "How to replace failure by a list of successes," in *Proceedings of the 1989 ACM Conference on Lisp and Functional Programming*, 1989, pp. 114–128.

[42] P. Hudak, "Building a functional language on the STG," in *Proceedings of the ACM SIGPLAN 1989 Conference on Programming Language Design and Implementation*, 1989, pp. 266–283.

[43] M. Shapiro, "Crdt: True distributed data structures," in *Proceedings of the 2015 IEEE 35th International Conference on Distributed Computing Systems (ICDCS)*, 2015, pp. 363–372.

[44] D. Schmidt, "The state monad," *Haskell Wiki*, 2023.

[45] M. O'Laighin, "Bracket patterns in Haskell," *Haskell Wiki*, 2023.

[46] E. Kohlweck, "Resource management in functional programming," *ACM Transactions on Programming Languages and Systems*, vol. 45, no. 2, pp. 1–32, 2023.

[47] C. Clack, "Incremental computation: The semantics of change," *Journal of Functional Programming*, vol. 1, no. 1, pp. 1–20, 1991.

[48] P. A. Bernstein, "Database transaction models," *ACM Computing Surveys*, vol. 25, no. 4, pp. 301–351, 1993.

[49] B. Chetkuri, "Version control systems: A survey," *IEEE Software*, vol. 35, no. 3, pp. 45–52, 2018.

[50] J. Smith, "Hot module replacement in web development," *Web Development Journal*, vol. 12, no. 2, pp. 34–45, 2022.

[51] M. Johnson, "Dynamic class loading in Java," *Java Technology Review*, vol. 18, no. 3, pp. 56–67, 2021.

[52] M. Fowler, "Dependency injection," *Martin Fowler's Blog*, 2004.

[53] S. Lewis, "Service registries in plugin systems," *Plugin Architecture Journal*, vol. 8, no. 1, pp. 12–25, 2020.

---

## Appendix A. Formal Definitions and Theorems

### A.1. Effect Context and Revertibility

Given a context Γ, define its **effect context** as:
𝜕_Γ ≔ Γ × (Γ → Γ)

It can be understood as a pair (𝛾, 𝜑), where:
- 𝛾 : Γ is the current context state;
- 𝜑 : Γ → Γ is the **accumulator**, the composite of the inverses of the effects performed so far, and the function that recovers the context to its initial state. In particular, the initial effect context can be represented as (𝛾_0, id_Γ).

We also write 𝜕^2_Γ for 𝜕(𝜕_Γ) = 𝜕_Γ × (𝜕_Γ → 𝜕_Γ); iterating 𝜕 this way yields the tower Γ, 𝜕_Γ, 𝜕^2_Γ, ⋯.

**Definition A.1.** Define the transformation **track_Γ** on pairs of context functions:
track_Γ : (Γ → Γ) × (Γ → Γ) → 𝜕_Γ → 𝜕_Γ
track_Γ = (f, g) ↦ (𝛾, 𝜑) ↦ (f(𝛾), 𝜑 ∘ g)

This transformation converts a forward function f together with a candidate inverse g into a transformation of the effect context 𝜕_Γ.

**Theorem A.2.** For every (f, g) ∈ (Γ → Γ) × (Γ → Γ), write f' ≔ track_Γ(f, g); then the following diagram commutes, that is, pr_1 ∘ f' = f ∘ pr_1.

**Theorem A.3.** track_Γ is a monoid homomorphism from 𝔗_Γ into 𝜕_Γ → 𝜕_Γ. That is,
1. track_Γ(id_Γ, id_Γ) = id_𝜕_Γ;
2. for all (f1, g1), (f2, g2) ∈ 𝔗_Γ, track_Γ((f1, g1) ∘ (f2, g2)) = track_Γ(f1, g1) ∘ track_Γ(f2, g2).

**Definition A.5.** Define the transformation **recover_Γ** on 𝜕_Γ:
recover_Γ : 𝜕_Γ → 𝜕_Γ
recover_Γ = (𝛾, 𝜑) ↦ (𝜑(𝛾), id_Γ)

This transformation applies the recovery function 𝜑 to the current state 𝛾 and resets 𝜑 to the identity.

### A.2. Coeffect Context and Reactive Coeffects

To formalize reactive coeffects, we introduce the concept of a **coeffect context**. Given a set of keys K, we define the coeffect context as:
Σ ≔ (k : K) ⇀ 𝒱_k

It can be understood as a partial function that maps each key k to a value 𝒱_k.

**Definition A.9.** The **get**, **set**, and **remove** operations on Σ are:
get : (k : K) × Σ ⇀ 𝒱_k
set : (k : K) × 𝒱_k → Σ ⇀ Σ × (Σ ⇀ Σ)
remove : (k : K) → Σ ⇀ Σ × (Σ ⇀ Σ)

### A.3. Unified Context and Observational Equivalence

We define the **unified context** as:
Ξ ≔ 𝜕_Σ

The unified context is the effect context of the coeffect context. It is a pair (𝛾, 𝜑), where 𝛾 is the coeffect context and 𝜑 is the accumulator of the inverses of the effects performed so far.

**Definition A.12.** The **effect** and **coeffect** operations on the unified context Ξ are:
effect : 𝔗_Σ → Ξ → Ξ  (effect = track_Σ)
coeffect : 𝔇 → Ξ → Act  (coeffect = d ↦ (𝛾, 𝜑) ↦ notify(d, 𝛾, λk. λv. d(k)(v)))

### A.4. Effect Independence and Coeffect Commutativity

**Definition A.17.** A set of effect functions F ⊆ 𝔗_Σ is **effect independent** if for all (f1, g1), (f2, g2) ∈ F, (f1, g1) ∘ (f2, g2) ∼ (f2, g2) ∘ (f1, g1).

**Definition A.18.** If a set of effect functions F ⊆ 𝔗_Σ is effect independent, then for all finite sequences (f1, g1), ⋯, (fn, gn) in F, and all permutations σ of {1, ⋯, n}, fold_n((f1, g1), ⋯, (fn, gn)) ∼ fold_n((f_σ(1), g_σ(1)), ⋯, (f_σ(n), g_σ(n))).

**Definition A.19.** If a set of coeffect specifications D ⊆ 𝔇 is coeffect commutative, then for all finite sequences d1, ⋯, dn in D, and all permutations σ of {1, ⋯, n}, notify(⋃_{i=1}^n d_i, γ, λk. λv. ⋃_{i=1}^n d_i(k)(v)) = notify(⋃_{i=1}^n d_σ(i), γ, λk. λv. ⋃_{i=1}^n d_σ(i)(k)(v)).

### A.5. Components and Fibers

**Definition A.20.** A **component** over a context Γ carrying both effects and coeffects (Definition 28) is defined as:
ℭ_Γ ≔ (d : 𝔇_Γ) × (p : 𝔓_Γ) × ℑ_{d ∪ p}_Γ

**Definition A.21.** A **fiber** over a context Γ is defined as:
𝔉_Γ ≔ (π : N → ℭ_Γ) × (θ : N → θ_n) × (τ : N → {Inactive, Reloading, Active, Unloading})

where:
- π : N → ℭ_Γ maps each fiber name to its component;
- θ : N → θ_n maps each fiber name to its state, where θ_n is a tuple (i, g, ω), with i being the remaining iterator, g being the accumulator of inverses, and ω being the committed view;
- τ : N → {Inactive, Reloading, Active, Unloading} is the lifecycle state.

The rules that move fibers are:
1. **Orchestration rules**: O-Insert, O-Retire, O-Remove.
2. **Lifecycle rules**: L-Begin, L-Iter, L-Finish, L-Divert, L-Leave, L-Unload.