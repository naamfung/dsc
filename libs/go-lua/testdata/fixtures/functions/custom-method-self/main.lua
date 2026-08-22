local obj = {
    value = 0,
    increment = function(self)
        self.value = self.value + 1
    end
}
obj:increment()
