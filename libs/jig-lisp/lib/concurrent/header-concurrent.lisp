;; $MODULE header-concurrent

(defmacro future (fn [& body]
    `(^{:once true} future-call (fn [] ~@body))))
