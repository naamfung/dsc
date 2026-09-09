;; uncomment to test

(map? (try
                1
                2
                3
                (throw true)
                (catch e
                    4
                    5
                    6
                    {
                        :err e
                        :desc "miserable error"
                    })
                (finally)))
