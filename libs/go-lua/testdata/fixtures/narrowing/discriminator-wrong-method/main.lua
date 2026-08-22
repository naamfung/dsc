type Dog = {kind: "dog", bark: () -> ()}
type Cat = {kind: "cat", meow: () -> ()}
type Animal = Dog | Cat

local function speak(a: Animal)
    if a.kind == "dog" then
        a.meow() -- expect-error
    end
end
